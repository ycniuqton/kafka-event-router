package routes_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/routestest"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/runtime/state/statetest"
	"github.com/eventrouter/router/internal/scheduler"
)

// lifecyclePort adapts routes' DeploymentLifecycle to route-runtime's port (as cmd/api wires it).
type lifecyclePort struct{ l *routes.DeploymentLifecycle }

func (p lifecyclePort) MarkActive(ctx context.Context, id string) error {
	_, err := p.l.MarkActive(ctx, routes.DeploymentID(id))
	return err
}

func (p lifecyclePort) MarkFailed(ctx context.Context, id, reason string) (bool, error) {
	out, err := p.l.MarkFailed(ctx, routes.DeploymentID(id), reason)
	return out.Paused, err
}

// notices records rollout notices (the WS bell once monitoring's bus exists).
type notices struct{ got []routes.RolloutNotice }

func (n *notices) Kind() routes.RolloutNotifierKind { return "test" }

func (n *notices) Notify(_ context.Context, notice routes.RolloutNotice) error {
	n.got = append(n.got, notice)
	return nil
}

// revertWorld is a route with live v1 and rolling v2 under route-runtime's rollout coordinator.
type revertWorld struct {
	core    *routes.Core
	pub     *routestest.Publisher
	mem     *statetest.Memory
	clock   time.Time
	notices *notices
	route   routes.Route
	v1, v2  routes.Activation
	rc      *scheduler.RolloutCoordinator
}

func newRevertWorld(t *testing.T) *revertWorld {
	t.Helper()
	core, pub := newCore(t)
	w := &revertWorld{core: core, pub: pub, mem: statetest.New(), clock: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC), notices: &notices{}}
	w.route, w.v1 = seedDeployed(t, core, false, nil)
	lifecycle := routes.NewDeploymentLifecycle(core)
	var err error
	if lifecycle.Notices, err = routes.NewRolloutNotifierManager("test", slog.New(slog.NewTextHandler(io.Discard, nil)), w.notices); err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.MarkActive(context.Background(), w.v1.Deployment.ID); err != nil {
		t.Fatal(err)
	}
	w.v2 = w.activate(t, json.RawMessage(`{"name":"r","v":2}`))
	w.mirror()
	w.rc = &scheduler.RolloutCoordinator{Stores: w.mem.Stores(), Lifecycle: lifecyclePort{l: lifecycle}, Timings: domain.DefaultTimings(), Now: func() time.Time { return w.clock }}
	return w
}

func (w *revertWorld) activate(t *testing.T, cfg json.RawMessage) routes.Activation {
	t.Helper()
	ctx := context.Background()
	route, _ := w.core.Repos.Routes.Get(ctx, w.route.ID)
	var act routes.Activation
	err := w.core.Tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		act, err = routes.VersionActivator{Repos: w.core.Repos}.Activate(ctx, routes.ActivationPlan{Route: route, Config: cfg, Origin: routes.OriginDeploy})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return act
}

// mirror copies the routes state into route-runtime's tables: v2 rolling, both replicas on v1.
func (w *revertWorld) mirror() {
	id := string(w.route.ID)
	w.mem.Demands[id] = state.RouteDemand{RouteID: id, DesiredState: "running", ActiveVersionID: string(w.v2.Version.ID), Replicas: 2}
	w.mem.Deployments[string(w.v2.Deployment.ID)] = &statetest.Deployment{Status: string(statetest.StatusRolling), Rollout: state.Rollout{
		DeploymentID: string(w.v2.Deployment.ID), RouteID: id, VersionID: string(w.v2.Version.ID), PreviousVersionID: string(w.v1.Version.ID)}}
	for _, worker := range []string{"w1", "w2"} {
		w.mem.SetLease(state.AssignmentRow{RouteID: id, WorkerID: worker, TargetVersionID: string(w.v1.Version.ID),
			ReportedVersionID: string(w.v1.Version.ID), RunnerState: domain.RunnerRunning})
	}
}

// report makes every replica targeting v2 report it in st.
func (w *revertWorld) report(st domain.RunnerState, message string) {
	for _, worker := range []string{"w1", "w2"} {
		a, _ := w.mem.Lease(string(w.route.ID), worker)
		if a.TargetVersionID == string(w.v2.Version.ID) {
			a.ReportedVersionID, a.RunnerState, a.ErrorMessage = a.TargetVersionID, st, message
			w.mem.SetLease(a)
		}
	}
}

// TestFailingRolloutAutoRollsBack (routes#p2-test-auto-revert): a replica erroring on v2 inside the 60 s window
// makes route-runtime call MarkFailed; routes creates v3 (origin auto_rollback, v1's config), switches the
// live version, emits route_changed and sends the rollback notice.
func TestFailingRolloutAutoRollsBack(t *testing.T) {
	w := newRevertWorld(t)
	ctx := context.Background()
	_ = w.rc.Advance(ctx)
	w.report(domain.RunnerError, "decode failed")
	if err := w.rc.Advance(ctx); err != nil {
		t.Fatal(err)
	}
	failed, _ := w.core.Repos.Deployments.Get(ctx, w.v2.Deployment.ID)
	if failed.Status != routes.DeployFailed || failed.FailureReason != "decode failed" {
		t.Fatalf("%+v", failed)
	}
	route, _ := w.core.Repos.Routes.Get(ctx, w.route.ID)
	v3, _ := w.core.Repos.Versions.GetByID(ctx, *route.ActiveVersionID)
	if v3.No != 3 || v3.Origin != routes.OriginAutoRollback || *v3.BasedOnNo != 1 || !(routes.ConfigFingerprint{}).Same(v3.Config, w.v1.Version.Config) {
		t.Fatalf("%+v", v3)
	}
	last := w.pub.Events[len(w.pub.Events)-1]
	if last.Kind != routes.ChangeAutoRollback || last.VersionID == nil || *last.VersionID != v3.ID {
		t.Fatalf("%+v", last)
	}
	if len(w.notices.got) != 1 || w.notices.got[0].Message != "Deploy v2 of r failed, rolled back as v3 (= v1)" {
		t.Fatalf("%+v", w.notices.got)
	}
	if w.mem.Runtime[string(w.route.ID)].HaltedAt != nil {
		t.Fatal("a rollback must not halt the route")
	}
}

// TestHealthyRolloutBecomesActiveAfterWindow: no error within the window → MarkActive, no new version.
func TestHealthyRolloutBecomesActiveAfterWindow(t *testing.T) {
	w := newRevertWorld(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		_ = w.rc.Advance(ctx)
		w.report(domain.RunnerRunning, "")
	}
	_ = w.rc.Advance(ctx)
	w.clock = w.clock.Add(domain.AutoRollbackWindow.D())
	if err := w.rc.Advance(ctx); err != nil {
		t.Fatal(err)
	}
	d, _ := w.core.Repos.Deployments.Get(ctx, w.v2.Deployment.ID)
	route, _ := w.core.Repos.Routes.Get(ctx, w.route.ID)
	if d.Status != routes.DeployActive || *route.ActiveVersionID != w.v2.Version.ID || len(w.notices.got) != 0 {
		t.Fatalf("%+v %+v", d, w.notices.got)
	}
}
