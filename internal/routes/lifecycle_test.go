package routes_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/routestest"
	"github.com/eventrouter/router/internal/routes/validation"
)

type passAll struct{}

func (passAll) Run(context.Context, validation.Request) validation.Result {
	return validation.Result{Valid: true}
}

func newCore(t *testing.T) (*routes.Core, *routestest.Publisher) {
	t.Helper()
	db := routestest.NewDB("team-1", "payments")
	pub := &routestest.Publisher{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &routes.Core{Repos: db.Repositories(), Tx: routestest.Tx{DB: db}, Validator: passAll{}, Events: pub,
		Guard: routes.TeamRoleWriteGuard{}, Log: log, Views: routes.SummaryBuilder{Log: log}}, pub
}

func seedDeployed(t *testing.T, core *routes.Core, needsApproval bool, requester *uuid.UUID) (routes.Route, routes.Activation) {
	t.Helper()
	ctx := context.Background()
	r, err := core.Repos.Routes.Create(ctx, routes.Route{TeamID: "team-1", Name: "r", DesiredState: routes.DesiredPaused, Replicas: 1, Draft: json.RawMessage(`{"name":"r"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var act routes.Activation
	err = core.Tx.WithinTx(ctx, func(ctx context.Context) error {
		act, err = routes.VersionActivator{Repos: core.Repos}.Activate(ctx, routes.ActivationPlan{Route: r, Config: r.Draft, Origin: routes.OriginDeploy, NeedsApproval: needsApproval, Actor: requester})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return r, act
}

func TestTransitionTable(t *testing.T) {
	tt := routes.TransitionTable{}
	allowed := [][2]routes.DeploymentStatus{
		{routes.DeployPendingApproval, routes.DeployRollingOut}, {routes.DeployPendingApproval, routes.DeployRejected},
		{routes.DeployPendingApproval, routes.DeploySuperseded}, {routes.DeployRollingOut, routes.DeployActive},
		{routes.DeployRollingOut, routes.DeployFailed}, {routes.DeployRollingOut, routes.DeploySuperseded},
	}
	for _, p := range allowed {
		if !tt.Allows(p[0], p[1]) {
			t.Errorf("%s → %s should be allowed", p[0], p[1])
		}
	}
	denied := [][2]routes.DeploymentStatus{
		{routes.DeployActive, routes.DeployFailed}, {routes.DeployFailed, routes.DeployActive},
		{routes.DeployPendingApproval, routes.DeployActive}, {routes.DeploySuperseded, routes.DeployRollingOut},
	}
	for _, p := range denied {
		if tt.Allows(p[0], p[1]) {
			t.Errorf("%s → %s should be denied", p[0], p[1])
		}
	}
}

func TestMarkFailedFirstDeployPauses(t *testing.T) {
	core, pub := newCore(t)
	r, act := seedDeployed(t, core, false, nil)
	out, err := routes.NewDeploymentLifecycle(core).MarkFailed(context.Background(), act.Deployment.ID, "boom")
	if err != nil || !out.Paused || out.Rollback != nil || out.Failed.FailureReason != "boom" {
		t.Fatalf("%+v %v", out, err)
	}
	got, _ := core.Repos.Routes.Get(context.Background(), r.ID)
	if got.DesiredState != routes.DesiredPaused || len(pub.Events) != 1 || pub.Events[0].Kind != routes.ChangePaused {
		t.Fatalf("%+v %+v", got, pub.Events)
	}
}

func TestApprovalActivateIsFourEyes(t *testing.T) {
	core, pub := newCore(t)
	requester := uuid.New()
	r, act := seedDeployed(t, core, true, &requester)
	if act.Deployment.Status != routes.DeployPendingApproval || act.Route.ActiveVersionID != nil {
		t.Fatalf("%+v", act)
	}
	l := routes.NewDeploymentLifecycle(core)
	if _, err := l.Activate(context.Background(), act.Deployment.ID, requester); !errors.Is(err, routes.ErrSelfApproval) {
		t.Fatalf("self approval: %v", err)
	}
	d, err := l.Activate(context.Background(), act.Deployment.ID, uuid.New())
	if err != nil || d.Status != routes.DeployRollingOut {
		t.Fatalf("%+v %v", d, err)
	}
	got, _ := core.Repos.Routes.Get(context.Background(), r.ID)
	if got.ActiveVersionID == nil || *got.ActiveVersionID != act.Version.ID || got.DesiredState != routes.DesiredRunning || len(pub.Events) != 1 {
		t.Fatalf("%+v", got)
	}
	if _, err := l.Reject(context.Background(), act.Deployment.ID, "late"); !errors.Is(err, routes.ErrTransitionNotAllowed) {
		t.Fatalf("reject after activation: %v", err)
	}
}

func TestNewDeploySupersedesOpenOne(t *testing.T) {
	core, _ := newCore(t)
	requester := uuid.New()
	r, first := seedDeployed(t, core, true, &requester)
	ctx := context.Background()
	err := core.Tx.WithinTx(ctx, func(ctx context.Context) error {
		_, err := routes.VersionActivator{Repos: core.Repos}.Activate(ctx, routes.ActivationPlan{Route: r, Config: json.RawMessage(`{"name":"r2"}`), Origin: routes.OriginDeploy})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := core.Repos.Deployments.Get(ctx, first.Deployment.ID)
	if d.Status != routes.DeploySuperseded {
		t.Fatalf("%+v", d)
	}
	if _, err := routes.NewDeploymentLifecycle(core).Activate(ctx, first.Deployment.ID, uuid.New()); !errors.Is(err, routes.ErrTransitionNotAllowed) {
		t.Fatalf("approval of superseded deploy: %v", err)
	}
}
