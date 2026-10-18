package routesport_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/approvals"
	"github.com/eventrouter/router/internal/approvals/approvalstest"
	"github.com/eventrouter/router/internal/approvals/routesport"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/rbac/rbactest"
	"github.com/eventrouter/router/internal/routes"
)

// gate answers a fixed outcome and remembers the target it was asked about.
type gate struct {
	approvals.Manager
	outcome approvals.GateOutcome
	target  approvals.DeployTarget
}

func (g *gate) Gate(_ context.Context, t approvals.DeployTarget) (approvals.GateResult, error) {
	g.target = t
	return approvals.GateResult{Outcome: g.outcome}, nil
}

func TestGatePolicyMapsOutcomes(t *testing.T) {
	conn := uuid.New()
	team := uuid.New()
	reader := approvals.DestinationReader{Connections: approvalstest.Connections{conn: {ID: conn, TeamID: team, EnvTag: "prod"}}}
	doc := []byte(`{"apiVersion":"router/v1","name":"r","source":{"connectionId":"x","topics":["a"]},"branches":[{"id":"main","destination":{"connectionId":"` + conn.String() + `","topic":"o"}}]}`)
	route := routes.Route{ID: routes.RouteID(uuid.NewString()), TeamID: routes.TeamID(team.String())}
	cases := map[approvals.GateOutcome]bool{approvals.OutcomeDeployNow: false, approvals.OutcomeRequireApproval: true}
	for outcome, want := range cases {
		g := &gate{outcome: outcome}
		got, err := routesport.GatePolicy{Gate: g, Destinations: reader}.Requires(context.Background(), route, doc)
		if err != nil || got != want {
			t.Fatalf("%s: %v %v", outcome, got, err)
		}
		if g.target.TeamID != team || len(g.target.Destinations) != 1 || !g.target.HasProd() {
			t.Fatalf("target = %+v", g.target)
		}
	}
	_, err := routesport.GatePolicy{Gate: &gate{outcome: approvals.OutcomeDeny}, Destinations: reader}.Requires(context.Background(), route, doc)
	if d := problem.NewCatalog().Resolve(err); d.Status != 403 || d.Code != string(routesport.CodeDeployNotAllowed) {
		t.Fatalf("deny = %+v", d)
	}
	if (routesport.GatePolicy{}).Kind() != routesport.PolicyKind {
		t.Fatal("kind")
	}
}

func TestWriteGuardUsesPolicyEngine(t *testing.T) {
	w := rbactest.NewWorld()
	team := uuid.New()
	approver, viewer, admin := w.AddUser(false, rbac.UserActive), w.AddUser(false, rbac.UserActive), w.AddUser(true, rbac.UserActive)
	w.Bind(approver, team, rbac.RoleApprover)
	w.Bind(viewer, team, rbac.RoleViewer)
	access := rbac.NewManager(rbac.ManagerDeps{Engine: rbac.NewRoleBindingEngine(rbac.DefaultRoleMatrix(), rbac.DefaultPermissionCatalog()),
		Subjects: rbac.NewSubjectResolver(rbac.NewTTLSubjectCache(0, nil), w), Matrix: rbac.DefaultRoleMatrix(), Catalog: rbac.DefaultPermissionCatalog(),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	g := routesport.WriteGuard{Access: access}
	cases := []struct {
		actor routes.Actor
		team  string
		want  bool
	}{
		{routes.Actor{UserID: &approver}, team.String(), true},
		{routes.Actor{UserID: &viewer}, team.String(), false},
		{routes.Actor{UserID: &admin}, uuid.NewString(), true},
		{routes.Actor{UserID: &approver}, uuid.NewString(), false},
		{routes.Actor{System: true}, "x", true},
		{routes.Actor{}, team.String(), false},
	}
	for i, c := range cases {
		if got := g.CanWrite(c.actor, routes.TeamID(c.team)); got != c.want {
			t.Errorf("case %d: %v", i, got)
		}
	}
}
