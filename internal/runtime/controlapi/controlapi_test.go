package controlapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/runtime/changes"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/runtime/state/statetest"
	"github.com/eventrouter/router/internal/store"
)

var t0 = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

type fakeLifecycle struct{ failed []string }

func (f *fakeLifecycle) MarkActive(context.Context, string) error { return nil }
func (f *fakeLifecycle) MarkFailed(_ context.Context, id, _ string) (bool, error) {
	f.failed = append(f.failed, id)
	return false, nil
}

type fakePauser struct{ paused []string }

func (p *fakePauser) Pause(_ context.Context, id string) error {
	p.paused = append(p.paused, id)
	return nil
}

type fixture struct {
	mem    *statetest.Memory
	router chi.Router
	life   *fakeLifecycle
	pause  *fakePauser
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	mem := statetest.New()
	now := func() time.Time { return t0 }
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := api.NewResponder(problem.NewCatalog(), log)
	stores := mem.Stores()
	agg := &RuntimeStatusAggregator{Assignments: stores.Assignments, Runtime: stores.Runtime}
	f := &fixture{mem: mem, life: &fakeLifecycle{}, pause: &fakePauser{}}
	internal := NewInternalModule(InternalDeps{
		Workers: &WorkerAPI{Stores: stores, Aggregator: agg, Timings: domain.DefaultTimings(), Now: now},
		Halts:   &HaltService{Stores: stores, Lifecycle: f.life, Pauser: f.pause, Log: log, Now: now},
		Changes: changes.NewBroker(), Responder: responder,
	})
	public := NewPublicModule(PublicDeps{Stores: stores, Responder: responder, Pages: api.NewPageParser(store.NewCursorCodec("k")),
		Timings: domain.DefaultTimings(), Now: now})
	r := chi.NewRouter()
	api.NewMounter(responder, nil, nil).Mount(r, internal, public)
	f.router = r
	return f
}

func (f *fixture) do(t *testing.T, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func TestHeartbeatExtendsLeasesAndDropsRefused(t *testing.T) {
	f := newFixture(t)
	f.mem.SetLease(state.AssignmentRow{RouteID: "r1", WorkerID: "w1", TargetVersionID: "v1", LeaseExpiresAt: t0})
	f.mem.SetLease(state.AssignmentRow{RouteID: "r2", WorkerID: "w1", TargetVersionID: "v1", LeaseExpiresAt: t0})
	rec := f.do(t, http.MethodPost, "/internal/v1/workers/w1/heartbeat", `{"host":"h","zone":"z","version":"1","capacity":10,"capacityUsed":1,"refusedRouteIds":["r2"],"draining":false}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body)
	}
	var reply domain.HeartbeatReply
	_ = json.Unmarshal(rec.Body.Bytes(), &reply)
	if reply.LeaseTTLMs != domain.LeaseTTL.D().Milliseconds() || reply.AssignmentsETag == "" {
		t.Fatalf("reply %+v", reply)
	}
	a, ok := f.mem.Lease("r1", "w1")
	if !ok || !a.LeaseExpiresAt.Equal(t0.Add(domain.LeaseTTL.D())) {
		t.Fatalf("lease %+v", a)
	}
	if _, ok := f.mem.Lease("r2", "w1"); ok {
		t.Fatal("refused lease kept")
	}
	if w := f.mem.Workers["w1"]; w.Zone != "z" || w.Capacity != 10 {
		t.Fatalf("worker %+v", w)
	}
}

func TestAssignmentsETagAndNotModified(t *testing.T) {
	f := newFixture(t)
	f.mem.RouteNames["r1"] = "orders"
	f.mem.VersionNos["v1"] = 3
	f.mem.SetLease(state.AssignmentRow{RouteID: "r1", WorkerID: "w1", TargetVersionID: "v1", LeaseExpiresAt: t0})
	rec := f.do(t, http.MethodGet, "/internal/v1/workers/w1/assignments", "", nil)
	var set domain.AssignmentSet
	_ = json.Unmarshal(rec.Body.Bytes(), &set)
	if rec.Code != http.StatusOK || len(set.Assignments) != 1 || set.Assignments[0].VersionNo != 3 || rec.Header().Get("ETag") != set.ETag {
		t.Fatalf("status %d set %+v", rec.Code, set)
	}
	rec = f.do(t, http.MethodGet, "/internal/v1/workers/w1/assignments", "", map[string]string{"If-None-Match": set.ETag})
	if rec.Code != http.StatusNotModified {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestStatusReportAggregatesRuntime(t *testing.T) {
	f := newFixture(t)
	f.mem.Demands["r1"] = state.RouteDemand{RouteID: "r1"}
	f.mem.VersionNos["v1"] = 4
	f.mem.SetLease(state.AssignmentRow{RouteID: "r1", WorkerID: "w1", TargetVersionID: "v1"})
	f.mem.SetLease(state.AssignmentRow{RouteID: "r1", WorkerID: "w2", TargetVersionID: "v1"})
	body := `{"replicas":[{"routeId":"r1","versionId":"v1","versionNo":4,"state":"running","lag":5,"partitions":3,"cpuMs":1,"memBytes":2}]}`
	if rec := f.do(t, http.MethodPost, "/internal/v1/workers/w1/status", body, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("status %d %s", rec.Code, rec.Body)
	}
	rt := f.mem.Runtime["r1"]
	if rt.Observed != domain.ObservedRunning || rt.Lag != 5 || rt.Partitions != 3 || rt.RunningVersionID != "v1" || len(rt.AssignedWorkers) != 2 {
		t.Fatalf("runtime %+v", rt)
	}
	rec := f.do(t, http.MethodGet, "/api/v1/routes/r1/runtime", "", nil)
	var view RuntimeView
	_ = json.Unmarshal(rec.Body.Bytes(), &view)
	if rec.Code != http.StatusOK || view.ObservedState != domain.ObservedRunning || len(view.Replicas) != 2 {
		t.Fatalf("view %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, http.MethodGet, "/api/v1/routes/nope/runtime", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown route %d", rec.Code)
	}
}

func TestHaltPausesRouteOrFailsRollout(t *testing.T) {
	f := newFixture(t)
	f.mem.Demands["r1"] = state.RouteDemand{RouteID: "r1", ActiveVersionID: "v1"}
	rec := f.do(t, http.MethodPost, "/internal/v1/routes/r1/halt", `{"workerId":"w1","versionId":"v1","reason":"error_policy_pause","errorCategory":"decode","message":"bad json"}`, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d %s", rec.Code, rec.Body)
	}
	if len(f.pause.paused) != 1 || f.mem.Runtime["r1"].HaltedAt == nil || f.mem.Runtime["r1"].HaltReason != domain.HaltErrorPolicyPause {
		t.Fatalf("paused %v runtime %+v", f.pause.paused, f.mem.Runtime["r1"])
	}
	f.mem.Demands["r2"] = state.RouteDemand{RouteID: "r2", ActiveVersionID: "v2"}
	f.mem.Deployments["d2"] = &statetest.Deployment{Rollout: state.Rollout{DeploymentID: "d2", RouteID: "r2", VersionID: "v2"}, Status: string(statetest.StatusRolling)}
	f.do(t, http.MethodPost, "/internal/v1/routes/r2/halt", `{"workerId":"w1","versionId":"v2","reason":"destination_permanent","errorCategory":"destination_permanent","message":"gone"}`, nil)
	if len(f.life.failed) != 1 || f.mem.Runtime["r2"].HaltedAt != nil || len(f.pause.paused) != 1 {
		t.Fatalf("rollout halt: failed %v runtime %+v", f.life.failed, f.mem.Runtime["r2"])
	}
}

func TestHaltOfSupersededVersionIsIgnored(t *testing.T) {
	f := newFixture(t)
	f.mem.Demands["r3"] = state.RouteDemand{RouteID: "r3", ActiveVersionID: "v3-rollback"}
	rec := f.do(t, http.MethodPost, "/internal/v1/routes/r3/halt", `{"workerId":"w2","versionId":"v2-failed","reason":"error_policy_pause","errorCategory":"step","message":"boom"}`, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d %s", rec.Code, rec.Body)
	}
	if len(f.pause.paused) != 0 || f.mem.Runtime["r3"].HaltedAt != nil {
		t.Fatalf("late halt of a rolled-back version paused the route: paused %v runtime %+v", f.pause.paused, f.mem.Runtime["r3"])
	}
}

func TestWorkersPage(t *testing.T) {
	f := newFixture(t)
	for _, id := range []string{"w1", "w2", "w3"} {
		f.mem.Workers[id] = state.Worker{ID: id, LastHeartbeatAt: t0}
	}
	rec := f.do(t, http.MethodGet, "/api/v1/workers?limit=2", "", nil)
	var page api.PageResponse[WorkerItem]
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if rec.Code != http.StatusOK || len(page.Items) != 2 || page.NextCursor == nil || !page.Items[0].Alive {
		t.Fatalf("page %d %s", rec.Code, rec.Body)
	}
	rec = f.do(t, http.MethodGet, "/api/v1/workers?limit=2&cursor="+*page.NextCursor, "", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Items) != 1 || page.Items[0].ID != "w3" || page.NextCursor != nil {
		t.Fatalf("second page %s", rec.Body)
	}
}

func TestReplicaFold(t *testing.T) {
	rows := []state.AssignmentRow{{WorkerID: "a", RunnerState: domain.RunnerRunning, ReportedVersionID: "v1", Lag: 2},
		{WorkerID: "b", RunnerState: domain.RunnerError, ErrorMessage: "boom"}, {WorkerID: "c"}}
	got := ReplicaFold{}.Fold("r", rows, false)
	if got.Observed != domain.ObservedError || got.ErrorMessage != "boom" || got.Lag != 2 || len(got.AssignedWorkers) != 3 {
		t.Fatalf("fold %+v", got)
	}
	if (ReplicaFold{}).Fold("r", nil, false).Observed != domain.ObservedPaused || (ReplicaFold{}).Fold("r", nil, true).Observed != domain.ObservedError {
		t.Fatal("empty fold")
	}
}
