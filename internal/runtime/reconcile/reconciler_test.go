package reconcile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/runtime/cache"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/internal/runtime/retry"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
)

type fakeCP struct {
	mu       sync.Mutex
	down     bool
	set      domain.AssignmentSet
	statuses [][]domain.ReplicaStatus
	halts    []domain.HaltReport
	hbs      []domain.Heartbeat
}

var errDown = errors.New("connection refused")

func (f *fakeCP) Heartbeat(_ context.Context, hb domain.Heartbeat) (domain.HeartbeatReply, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return domain.HeartbeatReply{}, errDown
	}
	f.hbs = append(f.hbs, hb)
	return domain.HeartbeatReply{AssignmentsETag: f.set.ETag}, nil
}

func (f *fakeCP) Assignments(_ context.Context, etag string) (domain.AssignmentSet, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return domain.AssignmentSet{}, false, errDown
	}
	return f.set, etag != "" && etag == f.set.ETag, nil
}

func (f *fakeCP) VersionConfig(context.Context, string) ([]byte, error) { return []byte(`{}`), nil }

func (f *fakeCP) ReportStatus(_ context.Context, s []domain.ReplicaStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return errDown
	}
	f.statuses = append(f.statuses, s)
	return nil
}

func (f *fakeCP) ReportHalt(_ context.Context, h domain.HaltReport) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return errDown
	}
	f.halts = append(f.halts, h)
	return nil
}

func (f *fakeCP) Changes(ctx context.Context) (<-chan domain.ChangeHint, error) {
	return nil, errDown
}

type fakeRunners struct {
	current map[string]runner.Current
	failed  map[string]domain.ErrorCategory
	ops     []string
	used    float64
}

func newFakeRunners() *fakeRunners {
	return &fakeRunners{current: map[string]runner.Current{}, failed: map[string]domain.ErrorCategory{}}
}

func (f *fakeRunners) Start(_ context.Context, v runner.CompiledVersion) error {
	f.ops = append(f.ops, "start:"+v.Ref.VersionID)
	f.current[v.Ref.RouteID] = runner.Current{Version: v, Ref: v.Ref, State: domain.RunnerRunning}
	return nil
}
func (f *fakeRunners) Swap(_ context.Context, v runner.CompiledVersion) error {
	f.ops = append(f.ops, "swap:"+v.Ref.VersionID)
	f.current[v.Ref.RouteID] = runner.Current{Version: v, Ref: v.Ref}
	return nil
}
func (f *fakeRunners) Restart(_ context.Context, v runner.CompiledVersion) error {
	f.ops = append(f.ops, "restart:"+v.Ref.VersionID)
	f.current[v.Ref.RouteID] = runner.Current{Version: v, Ref: v.Ref}
	return nil
}
func (f *fakeRunners) Remove(_ context.Context, id string, _ domain.StopReason) {
	f.ops = append(f.ops, "remove:"+id)
	delete(f.current, id)
}
func (f *fakeRunners) RecordFailure(ref domain.VersionRef, c domain.ErrorCategory, _ string) {
	f.failed[ref.RouteID] = c
	f.current[ref.RouteID] = runner.Current{Ref: ref, Failed: true}
}
func (f *fakeRunners) RecordUpgradeFailure(ref domain.VersionRef, c domain.ErrorCategory, msg string) {
	f.RecordFailure(ref, c, msg)
}
func (f *fakeRunners) Current(id string) (runner.Current, bool) { c, ok := f.current[id]; return c, ok }
func (f *fakeRunners) RouteIDs() []string {
	out := []string{}
	for id := range f.current {
		out = append(out, id)
	}
	return out
}
func (f *fakeRunners) Snapshot() []domain.ReplicaStatus { return nil }
func (f *fakeRunners) CapacityUsed() float64            { return f.used }
func (f *fakeRunners) StopAll(context.Context)          { f.current = map[string]runner.Current{} }

type fakeCompiler struct {
	configs map[string]*config.RouteConfig
	broken  map[string]bool
}

func (c fakeCompiler) Compile(_ context.Context, a domain.Assignment, _ []byte) (runner.CompiledVersion, error) {
	if c.broken[a.VersionID] {
		return runner.CompiledVersion{}, &CompileError{Cause: errors.New("bad filter")}
	}
	return runner.CompiledVersion{Ref: a.Ref(), Config: c.configs[a.VersionID]}, nil
}

func routeConfig(topic, destination string) *config.RouteConfig {
	return &config.RouteConfig{Name: "r", Source: config.Source{ConnectionID: "c1", Topics: []string{topic}},
		Branches: []config.Branch{{ID: "main", Destination: config.Destination{ConnectionID: "c1", Topic: destination}}}}
}

func newReconciler(t *testing.T, cp *fakeCP, rs *fakeRunners, comp fakeCompiler) *Reconciler {
	t.Helper()
	c, err := cache.NewFileConfigCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{ControlPlane: cp, Cache: c, Runners: rs, Compiler: comp, Reload: ConfigReloadClassifier{}, Halts: NewHaltQueue(),
		Identity: domain.WorkerIdentity{ID: "w1", Capacity: 10}, Timings: domain.DefaultTimings(), Retry: retry.DefaultPolicy(),
		Metrics: NewMetrics(nil), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

func assignment(route, version string) domain.Assignment {
	return domain.Assignment{RouteID: route, VersionID: version, RouteName: route}
}

func TestReconcileStartsUpgradesAndStops(t *testing.T) {
	cp := &fakeCP{set: domain.AssignmentSet{ETag: "1", Assignments: []domain.Assignment{assignment("r1", "v1")}}}
	rs := newFakeRunners()
	comp := fakeCompiler{configs: map[string]*config.RouteConfig{"v1": routeConfig("in", "out"), "v2": routeConfig("in", "out2"), "v3": routeConfig("other", "out2")}}
	r := newReconciler(t, cp, rs, comp)
	ctx := context.Background()
	r.Reconcile(ctx)
	comp.configs["v2"].Branches[0].Filter = &config.FilterNode{Type: "cond"}
	cp.set = domain.AssignmentSet{ETag: "2", Assignments: []domain.Assignment{assignment("r1", "v2")}}
	r.Reconcile(ctx)
	cp.set = domain.AssignmentSet{ETag: "3", Assignments: []domain.Assignment{assignment("r1", "v3")}}
	r.Reconcile(ctx)
	cp.set = domain.AssignmentSet{ETag: "4"}
	r.Reconcile(ctx)
	want := []string{"start:v1", "swap:v2", "restart:v3", "remove:r1"}
	if len(rs.ops) != len(want) {
		t.Fatalf("ops %v want %v", rs.ops, want)
	}
	for i := range want {
		if rs.ops[i] != want[i] {
			t.Fatalf("ops %v want %v", rs.ops, want)
		}
	}
}

func TestReconcileKeepsRunnersWhileControlPlaneDown(t *testing.T) {
	cp := &fakeCP{set: domain.AssignmentSet{ETag: "1", Assignments: []domain.Assignment{assignment("r1", "v1")}}}
	rs := newFakeRunners()
	r := newReconciler(t, cp, rs, fakeCompiler{configs: map[string]*config.RouteConfig{"v1": routeConfig("in", "out")}})
	ctx := context.Background()
	r.Reconcile(ctx)
	cp.down = true
	r.Reconcile(ctx)
	if !r.Degraded() {
		t.Fatal("expected degraded")
	}
	if _, ok := rs.Current("r1"); !ok {
		t.Fatal("runner stopped on silence")
	}
	rs.Halted(r)
	cp.down = false
	cp.set = domain.AssignmentSet{ETag: "2"}
	r.Reconcile(ctx)
	if _, ok := rs.Current("r1"); ok {
		t.Fatal("runner kept after explicit answer without lease")
	}
	if len(cp.halts) != 1 {
		t.Fatalf("halt not resent after reconnect: %d", len(cp.halts))
	}
}

// Halted queues a halt while the control plane is down.
func (f *fakeRunners) Halted(r *Reconciler) {
	r.d.Halts.Halted("r1", domain.VersionRef{RouteID: "r1", VersionID: "v1"}, &failures.HaltError{Reason: domain.HaltErrorPolicyPause})
}

func TestReconcileRecordsCompileFailureOnce(t *testing.T) {
	cp := &fakeCP{set: domain.AssignmentSet{ETag: "1", Assignments: []domain.Assignment{assignment("r1", "v1")}}}
	rs := newFakeRunners()
	r := newReconciler(t, cp, rs, fakeCompiler{broken: map[string]bool{"v1": true}})
	r.Reconcile(context.Background())
	r.Reconcile(context.Background())
	if rs.failed["r1"] != domain.CategoryCompile || len(rs.ops) != 0 {
		t.Fatalf("failed %v ops %v", rs.failed, rs.ops)
	}
}

func TestReconcileRefusesAboveCapacity(t *testing.T) {
	cp := &fakeCP{set: domain.AssignmentSet{ETag: "1", Assignments: []domain.Assignment{assignment("r1", "v1")}}}
	rs := newFakeRunners()
	rs.used = 9.5
	r := newReconciler(t, cp, rs, fakeCompiler{configs: map[string]*config.RouteConfig{"v1": routeConfig("in", "out")}})
	r.Reconcile(context.Background())
	r.heartbeat(context.Background())
	if len(rs.ops) != 0 {
		t.Fatalf("started above capacity: %v", rs.ops)
	}
	if hb := cp.hbs[len(cp.hbs)-1]; len(hb.RefusedRouteIDs) != 1 || hb.RefusedRouteIDs[0] != "r1" {
		t.Fatalf("heartbeat %+v", hb)
	}
}

func TestReloadClassifier(t *testing.T) {
	c := ConfigReloadClassifier{}
	base := routeConfig("in", "out")
	same := routeConfig("in", "out")
	swap := routeConfig("in", "out")
	swap.Branches[0].Filter = &config.FilterNode{Type: "cond"}
	topicChange := routeConfig("in", "out-renamed")
	sourceChange := routeConfig("other", "out")
	connChange := routeConfig("in", "out")
	connChange.Branches[0].Destination.ConnectionID = "c2"
	mode := config.DeliveryModeExactlyOnce
	modeChange := routeConfig("in", "out")
	modeChange.Delivery = &config.Delivery{Mode: mode}
	cases := []struct {
		next *config.RouteConfig
		want domain.ReloadKind
	}{
		{same, domain.ReloadNone}, {swap, domain.ReloadPipelineSwap}, {topicChange, domain.ReloadPipelineSwap},
		{sourceChange, domain.ReloadFullRestart}, {connChange, domain.ReloadFullRestart}, {modeChange, domain.ReloadFullRestart},
	}
	for i, tc := range cases {
		if got := c.Classify(base, tc.next); got != tc.want {
			t.Errorf("case %d: got %s want %s", i, got, tc.want)
		}
	}
}

// slowRunners starts each runner only after a delay, like real starts that open clients and join groups.
type slowRunners struct {
	*fakeRunners
	mu    sync.Mutex
	delay time.Duration
}

func (s *slowRunners) Start(ctx context.Context, v runner.CompiledVersion) error {
	time.Sleep(s.delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fakeRunners.Start(ctx, v)
}

func (s *slowRunners) RouteIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fakeRunners.RouteIDs()
}

func TestHeartbeatsContinueDuringLongReconcile(t *testing.T) {
	var as []domain.Assignment
	configs := map[string]*config.RouteConfig{}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("r%d", i)
		as = append(as, assignment(id, "v-"+id))
		configs["v-"+id] = routeConfig("in-"+id, "out-"+id)
	}
	cp := &fakeCP{set: domain.AssignmentSet{ETag: "1", Assignments: as}}
	rs := &slowRunners{fakeRunners: newFakeRunners(), delay: 25 * time.Millisecond}
	c, err := cache.NewFileConfigCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	timings := domain.DefaultTimings()
	timings.Heartbeat = 20 * time.Millisecond
	r := New(Deps{ControlPlane: cp, Cache: c, Runners: rs, Compiler: fakeCompiler{configs: configs}, Reload: ConfigReloadClassifier{},
		Halts: NewHaltQueue(), Identity: domain.WorkerIdentity{ID: "w1", Capacity: 100}, Timings: timings, Retry: retry.DefaultPolicy(),
		Metrics: NewMetrics(nil), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	time.Sleep(400 * time.Millisecond)
	cp.mu.Lock()
	beats := len(cp.hbs)
	cp.mu.Unlock()
	cancel()
	<-done
	if beats < 10 {
		t.Fatalf("%d heartbeats during a 500 ms reconciliation with a 20 ms interval; lease renewal starved", beats)
	}
}
