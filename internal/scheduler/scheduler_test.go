package scheduler

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/runtime/changes"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/runtime/state/statetest"
)

var t0 = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

type nopRefresher struct{}

func (nopRefresher) Refresh(context.Context, string) error { return nil }

type fakeLifecycle struct {
	mu      sync.Mutex
	active  []string
	failed  []string
	noPrior bool
}

func (f *fakeLifecycle) MarkActive(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active = append(f.active, id)
	return nil
}

func (f *fakeLifecycle) MarkFailed(_ context.Context, id, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = append(f.failed, id)
	return f.noPrior, nil
}

func worker(id, zone string, at time.Time) state.Worker {
	return state.Worker{ID: id, Host: id, Zone: zone, Capacity: 100, LastHeartbeatAt: at}
}

func planner(t *testing.T, m *statetest.Memory, c *clock, kind domain.PlacementKind) *AssignmentPlanner {
	t.Helper()
	pm, err := NewDefaultPlacementManager(kind)
	if err != nil {
		t.Fatal(err)
	}
	return &AssignmentPlanner{Stores: m.Stores(), Placement: pm, Runtime: nopRefresher{}, Timings: domain.DefaultTimings(), Now: c.Now}
}

func leasesOf(m *statetest.Memory, route string) []state.AssignmentRow {
	rows, _ := m.Stores().Assignments.ForRoute(context.Background(), route)
	return rows
}

func TestLeastLoadedSpreadsAcrossZonesAndRespectsCapacity(t *testing.T) {
	pool := []WorkerLoad{
		{Worker: state.Worker{ID: "a1", Host: "a1", Zone: "a", Capacity: 10}, Used: 1},
		{Worker: state.Worker{ID: "a2", Host: "a2", Zone: "a", Capacity: 10}, Used: 0},
		{Worker: state.Worker{ID: "b1", Host: "b1", Zone: "b", Capacity: 10}, Used: 5},
		{Worker: state.Worker{ID: "full", Host: "full", Zone: "c", Capacity: 1}, Used: 1},
		{Worker: state.Worker{ID: "drain", Host: "drain", Zone: "d", Capacity: 10, Draining: true}},
	}
	got := LeastLoadedStrategy{}.Place(state.RouteDemand{RouteID: "r"}, 2, pool, nil)
	if len(got) != 2 || got[0] != "a2" || got[1] != "b1" {
		t.Fatalf("placement %v, want [a2 b1] (zone spread before load)", got)
	}
	got = LeastLoadedStrategy{}.Place(state.RouteDemand{RouteID: "r"}, 1, pool, []state.AssignmentRow{{WorkerID: "a2"}})
	if len(got) != 1 || got[0] != "b1" {
		t.Fatalf("second replica %v, want other zone b1", got)
	}
	if (LeastLoadedStrategy{}).Cap(3) != 3 || (SingleWorkerStrategy{}).Cap(3) != 1 {
		t.Fatal("caps")
	}
}

func TestPlannerReplicasPartitionCapAndPause(t *testing.T) {
	m := statetest.New()
	c := &clock{now: t0}
	for _, w := range []state.Worker{worker("w1", "a", t0), worker("w2", "b", t0), worker("w3", "c", t0)} {
		m.Workers[w.ID] = w
	}
	m.Demands["r1"] = state.RouteDemand{RouteID: "r1", DesiredState: "running", ActiveVersionID: "v1", Replicas: 2}
	m.Demands["r2"] = state.RouteDemand{RouteID: "r2", DesiredState: "running", ActiveVersionID: "v9", Replicas: 3, Partitions: 1}
	p := planner(t, m, c, domain.PlacementLeastLoaded)
	if err := p.Plan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(leasesOf(m, "r1")); n != 2 {
		t.Fatalf("r1 replicas %d", n)
	}
	if n := len(leasesOf(m, "r2")); n != 1 {
		t.Fatalf("r2 replicas %d (capped at partitions)", n)
	}
	for _, a := range leasesOf(m, "r1") {
		if a.TargetVersionID != "v1" || !a.LeaseExpiresAt.Equal(t0.Add(domain.LeaseTTL.D())) {
			t.Fatalf("lease %+v", a)
		}
	}
	d := m.Demands["r1"]
	d.DesiredState = "paused"
	m.Demands["r1"] = d
	_ = p.Plan(context.Background())
	if n := len(leasesOf(m, "r1")); n != 0 {
		t.Fatalf("paused route kept %d leases", n)
	}
}

func TestPlannerReplacesDeadAndDrainingWorkers(t *testing.T) {
	m := statetest.New()
	c := &clock{now: t0}
	m.Workers["dead"] = worker("dead", "a", t0.Add(-time.Minute))
	drain := worker("drain", "b", t0)
	drain.Draining = true
	m.Workers["drain"] = drain
	m.Workers["w1"] = worker("w1", "c", t0)
	m.Workers["w2"] = worker("w2", "d", t0)
	m.Demands["r1"] = state.RouteDemand{RouteID: "r1", DesiredState: "running", ActiveVersionID: "v1", Replicas: 2}
	m.SetLease(state.AssignmentRow{RouteID: "r1", WorkerID: "dead", TargetVersionID: "v1", LeaseExpiresAt: t0.Add(-time.Second)})
	m.SetLease(state.AssignmentRow{RouteID: "r1", WorkerID: "drain", TargetVersionID: "v1", LeaseExpiresAt: t0.Add(time.Minute)})
	if n, _ := (LeaseReaper{Leases: m.Stores().Assignments, Now: c.Now}).Reap(context.Background()); n != 1 {
		t.Fatalf("reaped %d", n)
	}
	if err := planner(t, m, c, domain.PlacementLeastLoaded).Plan(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, a := range leasesOf(m, "r1") {
		got[a.WorkerID] = true
	}
	if len(got) != 2 || !got["w1"] || !got["w2"] {
		t.Fatalf("leases on %v, want w1+w2", got)
	}
}

func TestPlannerHaltedRouteLosesLeasesUntilResumed(t *testing.T) {
	m := statetest.New()
	c := &clock{now: t0}
	m.Workers["w1"] = worker("w1", "a", t0)
	m.Demands["r1"] = state.RouteDemand{RouteID: "r1", DesiredState: "paused", ActiveVersionID: "v1", Replicas: 1, UpdatedAt: t0}
	m.SetLease(state.AssignmentRow{RouteID: "r1", WorkerID: "w1", TargetVersionID: "v1", LeaseExpiresAt: t0.Add(time.Minute)})
	halted := t0.Add(time.Second)
	m.Runtime["r1"] = state.RuntimeRow{RouteID: "r1", HaltedAt: &halted, HaltReason: domain.HaltErrorPolicyPause}
	p := planner(t, m, c, domain.PlacementSingleWorker)
	_ = p.Plan(context.Background())
	if len(leasesOf(m, "r1")) != 0 {
		t.Fatal("halted route kept its lease")
	}
	m.Demands["r1"] = state.RouteDemand{RouteID: "r1", DesiredState: "running", ActiveVersionID: "v1", Replicas: 1, UpdatedAt: t0.Add(time.Minute)}
	_ = p.Plan(context.Background())
	if len(leasesOf(m, "r1")) != 1 || m.Runtime["r1"].HaltedAt != nil {
		t.Fatalf("resume did not lift the halt: %+v", m.Runtime["r1"])
	}
}

func rolloutFixture(prior bool) (*statetest.Memory, *clock, *fakeLifecycle, *RolloutCoordinator) {
	m := statetest.New()
	c := &clock{now: t0}
	lc := &fakeLifecycle{noPrior: !prior}
	m.Demands["r1"] = state.RouteDemand{RouteID: "r1", DesiredState: "running", ActiveVersionID: "v2", Replicas: 2}
	m.Deployments["d2"] = &statetest.Deployment{Rollout: state.Rollout{DeploymentID: "d2", RouteID: "r1", VersionID: "v2", PreviousVersionID: "v1"},
		Status: string(statetest.StatusRolling)}
	for _, w := range []string{"w1", "w2"} {
		m.SetLease(state.AssignmentRow{RouteID: "r1", WorkerID: w, TargetVersionID: "v1", ReportedVersionID: "v1", RunnerState: domain.RunnerRunning})
	}
	return m, c, lc, &RolloutCoordinator{Stores: m.Stores(), Lifecycle: lc, Timings: domain.DefaultTimings(), Now: c.Now}
}

func report(m *statetest.Memory, worker, version string, st domain.RunnerState) {
	a, _ := m.Lease("r1", worker)
	a.ReportedVersionID, a.RunnerState = version, st
	m.SetLease(a)
}

func TestRolloutOneReplicaAtATimeThenActiveAfterWindow(t *testing.T) {
	m, c, lc, rc := rolloutFixture(true)
	ctx := context.Background()
	_ = rc.Advance(ctx)
	_ = rc.Advance(ctx)
	targets := 0
	for _, a := range leasesOf(m, "r1") {
		if a.TargetVersionID == "v2" {
			targets++
		}
	}
	if targets != 1 {
		t.Fatalf("%d replicas targeted at once", targets)
	}
	first := leasesOf(m, "r1")[0].WorkerID
	report(m, first, "v2", domain.RunnerRunning)
	_ = rc.Advance(ctx)
	for _, a := range leasesOf(m, "r1") {
		report(m, a.WorkerID, a.TargetVersionID, domain.RunnerRunning)
	}
	_ = rc.Advance(ctx)
	if m.Deployments["d2"].ActivatedAt == nil {
		t.Fatal("activated_at not stamped once all replicas converged")
	}
	_ = rc.Advance(ctx)
	if len(lc.active) != 0 {
		t.Fatal("marked active inside the auto-rollback window")
	}
	c.now = c.now.Add(domain.AutoRollbackWindow.D())
	_ = rc.Advance(ctx)
	if len(lc.active) != 1 || lc.active[0] != "d2" {
		t.Fatalf("active %v", lc.active)
	}
}

func TestRolloutErrorWithinWindowTriggersAutoRollback(t *testing.T) {
	m, _, lc, rc := rolloutFixture(true)
	ctx := context.Background()
	_ = rc.Advance(ctx)
	for _, a := range leasesOf(m, "r1") {
		if a.TargetVersionID == "v2" {
			report(m, a.WorkerID, "v2", domain.RunnerError)
		}
	}
	_ = rc.Advance(ctx)
	if len(lc.failed) != 1 || lc.failed[0] != "d2" {
		t.Fatalf("failed %v", lc.failed)
	}
	if m.Runtime["r1"].HaltedAt != nil {
		t.Fatal("a rollback must not halt the route")
	}
}

func TestRolloutFailureWithoutPreviousVersionHalts(t *testing.T) {
	m, _, _, rc := rolloutFixture(false)
	ctx := context.Background()
	_ = rc.Advance(ctx)
	for _, a := range leasesOf(m, "r1") {
		if a.TargetVersionID == "v2" {
			report(m, a.WorkerID, "v2", domain.RunnerError)
		}
	}
	_ = rc.Advance(ctx)
	if rt := m.Runtime["r1"]; rt.HaltedAt == nil || rt.HaltReason != domain.HaltAutoRollbackFailed {
		t.Fatalf("runtime %+v", rt)
	}
}

func TestLoopPassUnderAlwaysLeader(t *testing.T) {
	m := statetest.New()
	c := &clock{now: t0}
	m.Workers["w1"] = worker("w1", "a", t0)
	m.Demands["r1"] = state.RouteDemand{RouteID: "r1", DesiredState: "running", ActiveVersionID: "v1", Replicas: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loop := &SchedulerLoop{Elector: AlwaysLeader{}, Reaper: LeaseReaper{Leases: m.Stores().Assignments, Now: c.Now},
		Planner: planner(t, m, c, domain.PlacementLeastLoaded), Rollout: &RolloutCoordinator{Stores: m.Stores(), Lifecycle: &fakeLifecycle{}, Timings: domain.DefaultTimings(), Now: c.Now}}
	loop.Pass(ctx)
	if len(leasesOf(m, "r1")) != 1 {
		t.Fatal("pass did not place the route")
	}
}

func TestLeaderGateFollowsLeadership(t *testing.T) {
	m := statetest.New()
	c := &clock{now: t0}
	gate := &LeaderState{}
	ctx, cancel := context.WithCancel(context.Background())
	timings := domain.DefaultTimings()
	timings.Scheduler = 10 * time.Millisecond
	loop := &SchedulerLoop{Elector: AlwaysLeader{}, Reaper: LeaseReaper{Leases: m.Stores().Assignments, Now: c.Now},
		Planner: planner(t, m, c, domain.PlacementLeastLoaded), Rollout: &RolloutCoordinator{Stores: m.Stores(), Lifecycle: &fakeLifecycle{}, Timings: timings, Now: c.Now},
		Hints: changes.NewBroker(), Timings: timings, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), State: gate}
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !gate.IsLeader() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !gate.IsLeader() {
		t.Fatal("gate must report leadership while the loop leads")
	}
	cancel()
	<-done
	if gate.IsLeader() {
		t.Fatal("gate must drop leadership when the loop ends")
	}
	var none *LeaderState
	if none.IsLeader() {
		t.Fatal("a nil gate is never leader")
	}
}
