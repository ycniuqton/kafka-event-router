package routes_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/routestest"
)

// prodPolicy stands in for access-control's gate: documents writing to "prod-conn" need approval.
type prodPolicy struct{}

func (prodPolicy) Kind() routes.ApprovalPolicyKind { return routes.ApprovalProduction }

func (prodPolicy) Requires(_ context.Context, _ routes.Route, doc json.RawMessage) (bool, error) {
	return bytes.Contains(doc, []byte("prod-conn")), nil
}

// requests records approval requests routes registers after commit.
type requests struct{ got []routes.Deployment }

func (r *requests) RequestApproval(_ context.Context, d routes.Deployment) error {
	r.got = append(r.got, d)
	return nil
}

// approvalWorld is a deploy manager with the production policy over in-memory repositories.
type approvalWorld struct {
	ctx       context.Context
	deploy    *routes.DeployManager
	routes    *routes.RouteManager
	lifecycle *routes.DeploymentLifecycle
	pub       *routestest.Publisher
	requests  *requests
	requester uuid.UUID
	repos     routes.Repositories
}

func newApprovalWorld(t *testing.T) *approvalWorld {
	t.Helper()
	team := uuid.New()
	db := routestest.NewDB(routes.TeamID(team.String()), "payments")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := &approvalWorld{pub: &routestest.Publisher{}, requests: &requests{}, requester: uuid.New(), repos: db.Repositories()}
	core := &routes.Core{Repos: db.Repositories(), Tx: routestest.Tx{DB: db}, Validator: passAll{}, Events: w.pub,
		Guard: routes.TeamRoleWriteGuard{}, Log: log, Views: routes.SummaryBuilder{Log: log}}
	policy, err := routes.NewApprovalPolicyManager(routes.ApprovalAuto, prodPolicy{})
	if err != nil || policy.Kind() != routes.ApprovalProduction {
		t.Fatalf("auto must pick the production gate: %v", err)
	}
	w.deploy = routes.NewDeployManager(core, policy)
	w.deploy.Requests = w.requests
	w.routes, w.lifecycle = routes.NewRouteManager(core), routes.NewDeploymentLifecycle(core)
	w.ctx = auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.PrincipalUser, UserID: w.requester,
		Teams: []auth.TeamRole{{TeamID: team, Role: auth.RoleEditor}}})
	return w
}

// save replaces the draft (destination connection conn, filter amount) and returns the new revision.
func (w *approvalWorld) save(t *testing.T, id routes.RouteID, conn string, amount int) routes.Revision {
	t.Helper()
	r, _ := w.routes.Get(w.ctx, id)
	doc, _ := json.Marshal(map[string]any{"name": "orders", "amount": amount, "destination": conn})
	d, err := w.routes.Update(w.ctx, id, routes.RouteChange{Draft: doc}, r.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return d.Revision
}

// TestProductionDeployNeedsApproval (routes#p2-test-approval): prod deploy → pending_approval (approval
// requested, nothing live) → approve → rolling_out; reject keeps the version number; a superseded request → 409.
func TestProductionDeployNeedsApproval(t *testing.T) {
	w := newApprovalWorld(t)
	created, err := w.routes.Create(w.ctx, routes.NewRoute{Name: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.ID
	res, err := w.deploy.Deploy(w.ctx, id, w.save(t, id, "prod-conn", 1), "first")
	if err != nil || res.Deployment.Status != routes.DeployPendingApproval || res.Route.ActiveVersion != nil {
		t.Fatalf("%+v %v", res.Deployment, err)
	}
	if len(w.pub.Events) != 0 || len(w.requests.got) != 1 || w.requests.got[0].ID != res.Deployment.ID {
		t.Fatalf("events %d, requests %+v", len(w.pub.Events), w.requests.got)
	}
	if _, err := w.lifecycle.Activate(w.ctx, res.Deployment.ID, w.requester); !errors.Is(err, routes.ErrSelfApproval) {
		t.Fatalf("self approval: %v", err)
	}
	approved, err := w.lifecycle.Activate(w.ctx, res.Deployment.ID, uuid.New())
	if err != nil || approved.Status != routes.DeployRollingOut || len(w.pub.Events) != 1 {
		t.Fatalf("%+v %v", approved, err)
	}
	if live, _ := w.routes.Get(w.ctx, id); live.Status != routes.StatusRunning || live.ActiveVersion.No != 1 {
		t.Fatalf("%+v", live.RouteSummary)
	}
	w.rejectKeepsNumber(t, id)
	w.supersededIsConflict(t, id)
}

func (w *approvalWorld) rejectKeepsNumber(t *testing.T, id routes.RouteID) {
	t.Helper()
	pending, err := w.deploy.Deploy(w.ctx, id, w.save(t, id, "prod-conn", 2), "")
	if err != nil || pending.Version.No != 2 {
		t.Fatalf("%+v %v", pending.Version, err)
	}
	rejected, err := w.lifecycle.Reject(w.ctx, pending.Deployment.ID, "not now")
	if err != nil || rejected.Status != routes.DeployRejected {
		t.Fatalf("%+v %v", rejected, err)
	}
	direct, err := w.deploy.Deploy(w.ctx, id, w.save(t, id, "dev-conn", 3), "")
	if err != nil || direct.Version.No != 3 || direct.Deployment.Status != routes.DeployRollingOut {
		t.Fatalf("non-prod deploys directly and v2 stays consumed: %+v %v", direct.Version, err)
	}
}

func (w *approvalWorld) supersededIsConflict(t *testing.T, id routes.RouteID) {
	t.Helper()
	older, err := w.deploy.Deploy(w.ctx, id, w.save(t, id, "prod-conn", 4), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.deploy.Deploy(w.ctx, id, w.save(t, id, "prod-conn", 5), ""); err != nil {
		t.Fatal(err)
	}
	_, err = w.lifecycle.Activate(w.ctx, older.Deployment.ID, uuid.New())
	if d := problem.NewCatalog().Resolve(err); d.Status != 409 {
		t.Fatalf("approving a superseded request: %+v (%v)", d, err)
	}
	if len(w.requests.got) != 4 {
		t.Fatalf("every pending deploy is registered: %d", len(w.requests.got))
	}
}

// TestPendingRequestKeepsTheLiveRollout: a request waiting for approval must not retire the rollout of the
// version that is live; only the approval hands over (live rollout superseded, approved one rolling out).
func TestPendingRequestKeepsTheLiveRollout(t *testing.T) {
	w := newApprovalWorld(t)
	created, err := w.routes.Create(w.ctx, routes.NewRoute{Name: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.ID
	live, err := w.deploy.Deploy(w.ctx, id, w.save(t, id, "dev-conn", 1), "live")
	if err != nil || live.Deployment.Status != routes.DeployRollingOut {
		t.Fatalf("%+v %v", live.Deployment, err)
	}
	pending, err := w.deploy.Deploy(w.ctx, id, w.save(t, id, "prod-conn", 2), "needs approval")
	if err != nil || pending.Deployment.Status != routes.DeployPendingApproval {
		t.Fatalf("%+v %v", pending.Deployment, err)
	}
	w.expectStatus(t, live.Deployment.ID, routes.DeployRollingOut)
	if _, err := w.lifecycle.Activate(w.ctx, pending.Deployment.ID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	w.expectStatus(t, live.Deployment.ID, routes.DeploySuperseded)
	w.expectStatus(t, pending.Deployment.ID, routes.DeployRollingOut)
}

func (w *approvalWorld) expectStatus(t *testing.T, id routes.DeploymentID, want routes.DeploymentStatus) {
	t.Helper()
	if d, err := w.repos.Deployments.Get(w.ctx, id); err != nil || d.Status != want {
		t.Fatalf("deployment %s: %s (%v), want %s", id, d.Status, err, want)
	}
}

// TestAutoPolicyFallsBackToNone: without access-control's gate, auto never requires approval.
func TestAutoPolicyFallsBackToNone(t *testing.T) {
	m, err := routes.NewApprovalPolicyManager(routes.ApprovalAuto)
	if err != nil || m.Kind() != routes.ApprovalNone {
		t.Fatalf("%v %v", m, err)
	}
	if _, err := routes.NewApprovalPolicyManager(routes.ApprovalProduction); err == nil {
		t.Fatal("an explicit production policy without the gate must fail")
	}
}
