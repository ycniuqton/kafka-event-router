package approvals_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/approvals"
	"github.com/eventrouter/router/internal/approvals/approvalstest"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/rbac/rbactest"
	"github.com/eventrouter/router/internal/store"
)

var (
	teamA    = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	prodConn = uuid.MustParse("00000000-0000-0000-0000-0000000000c1")
	devConn  = uuid.MustParse("00000000-0000-0000-0000-0000000000c2")
)

// fixture is one workspace: an editor (requester), an approver, a viewer and a workspace admin on team A.
type fixture struct {
	world     *rbactest.World
	store     *approvalstest.Store
	activator *approvalstest.Activator
	notifier  *approvals.MemoryNotifier
	teams     approvalstest.Teams
	manager   *approvals.DefaultManager
	editor    uuid.UUID
	approver  uuid.UUID
	viewer    uuid.UUID
	admin     uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{world: rbactest.NewWorld(), store: approvalstest.NewStore(), notifier: &approvals.MemoryNotifier{},
		teams: approvalstest.Teams{teamA: {ID: teamA, RequireProdApproval: true}}}
	f.editor, f.approver = f.user(rbac.RoleEditor, false), f.user(rbac.RoleApprover, false)
	f.viewer, f.admin = f.user(rbac.RoleViewer, false), f.user(rbac.RoleAdmin, true)
	f.store.Approvals = []uuid.UUID{f.approver, f.admin}
	f.activator = &approvalstest.Activator{Store: f.store}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	access := rbac.NewManager(rbac.ManagerDeps{Engine: rbac.NewRoleBindingEngine(rbac.DefaultRoleMatrix(), rbac.DefaultPermissionCatalog()),
		Subjects: rbac.NewSubjectResolver(rbac.NewTTLSubjectCache(0, nil), f.world), Matrix: rbac.DefaultRoleMatrix(),
		Catalog: rbac.DefaultPermissionCatalog(), Log: log})
	conns := approvalstest.Connections{prodConn: {ID: prodConn, TeamID: teamA, Name: "prod", EnvTag: rbac.EnvProd},
		devConn: {ID: devConn, TeamID: teamA, Name: "dev", EnvTag: rbac.EnvDev}}
	f.manager = approvals.NewManager(approvals.Deps{Tx: rbactest.PassTx{}, Store: f.store, Access: access, Teams: f.teams,
		Destinations: approvals.DestinationReader{Connections: conns}, Policy: approvals.TeamProductionPolicy{}, Activator: f.activator,
		Notifier: f.notifier, Trail: rbac.NewAuditTrail(f.world), Metrics: approvals.NewMetrics(nil, f.store), Log: log})
	return f
}

func (f *fixture) user(role rbac.Role, workspaceAdmin bool) uuid.UUID {
	id := f.world.AddUser(workspaceAdmin, rbac.UserActive)
	f.world.Bind(id, teamA, role)
	return id
}

func (f *fixture) pending() uuid.UUID {
	return f.store.AddPending(teamA, f.editor)
}

func code(err error) string {
	d := problem.NewCatalog().Resolve(err)
	return d.Code
}

func TestTeamProductionPolicyBranches(t *testing.T) {
	prod := approvals.DeployTarget{Destinations: []rbac.Resource{{EnvTag: rbac.EnvDev}, {EnvTag: rbac.EnvProd}}}
	nonProd := approvals.DeployTarget{Destinations: []rbac.Resource{{EnvTag: rbac.EnvStaging}}}
	editor := approvals.CallerRights{DeployNonProd: true, Write: true}
	approver := approvals.CallerRights{DeployNonProd: true, DeployProd: true, Write: true}
	cases := []struct {
		name    string
		rights  approvals.CallerRights
		target  approvals.DeployTarget
		require bool
		want    approvals.GateOutcome
		reason  approvals.GateReason
	}{
		{"non-prod editor", editor, nonProd, true, approvals.OutcomeDeployNow, approvals.ReasonNonProd},
		{"non-prod viewer", approvals.CallerRights{}, nonProd, true, approvals.OutcomeDeny, approvals.ReasonMissingPermission},
		{"prod no policy approver", approver, prod, false, approvals.OutcomeDeployNow, approvals.ReasonProdDirect},
		{"prod no policy editor", editor, prod, false, approvals.OutcomeRequireApproval, approvals.ReasonProdApproval},
		{"prod policy approver (strict 4-eyes)", approver, prod, true, approvals.OutcomeRequireApproval, approvals.ReasonProdApproval},
		{"prod policy editor", editor, prod, true, approvals.OutcomeRequireApproval, approvals.ReasonProdApproval},
		{"prod viewer", approvals.CallerRights{}, prod, false, approvals.OutcomeDeny, approvals.ReasonMissingPermission},
		{"no destinations", editor, approvals.DeployTarget{}, true, approvals.OutcomeDeployNow, approvals.ReasonNonProd},
	}
	for _, c := range cases {
		got := approvals.TeamProductionPolicy{}.Evaluate(context.Background(), c.rights, c.target, approvals.TeamPolicy{RequireProdApproval: c.require})
		if got.Outcome != c.want || got.Reason != c.reason {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
}

func TestGateUsesRolesTeamPolicyAndApproverCount(t *testing.T) {
	f := newFixture(t)
	prod := approvals.DeployTarget{TeamID: teamA, Destinations: []rbac.Resource{{EnvTag: rbac.EnvProd}}}
	res, err := f.manager.Gate(approvalstest.As(context.Background(), f.editor), prod)
	if err != nil || res.Outcome != approvals.OutcomeRequireApproval || res.Approvers != 2 {
		t.Fatalf("editor prod = %+v %v", res, err)
	}
	f.store.Approvals = []uuid.UUID{f.editor}
	res, _ = f.manager.Gate(approvalstest.As(context.Background(), f.editor), prod)
	if res.Reason != approvals.ReasonNoApprover || res.Approvers != 0 {
		t.Fatalf("no approver = %+v", res)
	}
	f.teams[teamA] = rbac.Team{ID: teamA, RequireProdApproval: false}
	res, _ = f.manager.Gate(approvalstest.As(context.Background(), f.approver), prod)
	if res.Outcome != approvals.OutcomeDeployNow {
		t.Fatalf("approver without team policy = %+v", res)
	}
	res, _ = f.manager.Gate(approvalstest.As(context.Background(), f.viewer), prod)
	if res.Outcome != approvals.OutcomeDeny {
		t.Fatalf("viewer = %+v", res)
	}
	if _, err := f.manager.Gate(approvalstest.As(context.Background(), f.editor), approvals.DeployTarget{TeamID: uuid.New()}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown team: %v", err)
	}
}

func TestApproveStateMachine(t *testing.T) {
	f := newFixture(t)
	id := f.pending()
	if _, err := f.manager.Approve(approvalstest.As(context.Background(), f.editor), id, "", 1); code(err) != string(rbac.ReasonMissingPermission) {
		t.Fatalf("editor cannot approve: %v", err)
	}
	f.world.Bind(f.editor, teamA, rbac.RoleAdmin)
	f.world.Users[f.editor].WorkspaceAdmin = true
	if _, err := f.manager.Approve(approvalstest.As(context.Background(), f.editor), id, "", 1); code(err) != string(approvals.CodeSelfApproval) {
		t.Fatalf("no admin bypass: %v", err)
	}
	if _, err := f.manager.Approve(approvalstest.As(context.Background(), f.viewer), id, "", 1); code(err) != string(rbac.ReasonMissingPermission) {
		t.Fatalf("viewer: %v", err)
	}
	var stale *store.PreconditionFailedError
	if _, err := f.manager.Approve(approvalstest.As(context.Background(), f.approver), id, "", 7); !errors.As(err, &stale) {
		t.Fatalf("stale if-match: %v", err)
	}
	a, err := f.manager.Approve(approvalstest.As(context.Background(), f.approver), id, "ship it", 1)
	if err != nil || a.Status != approvals.StatusRollingOut || a.DecisionNote != "ship it" {
		t.Fatalf("approve = %+v %v", a, err)
	}
	if _, err := f.manager.Approve(approvalstest.As(context.Background(), f.admin), id, "", a.RowVersion); code(err) != string(approvals.CodeAlreadyDecided) {
		t.Fatalf("second decision: %v", err)
	}
	events := f.notifier.Events()
	if len(events) != 1 || events[0].Kind != approvals.EventDecided || events[0].Recipients[0] != f.editor {
		t.Fatalf("events = %+v", events)
	}
}

func TestApproveRevalidationFailureKeepsPending(t *testing.T) {
	f := newFixture(t)
	id := f.pending()
	f.activator.Err = problem.NewError(problem.KindValidation, "connection deleted")
	if _, err := f.manager.Approve(approvalstest.As(context.Background(), f.approver), id, "", 1); err == nil {
		t.Fatal("expected re-validation error")
	}
	if f.store.Items[id].Status != approvals.StatusPending || len(f.notifier.Events()) != 0 {
		t.Fatalf("request changed: %+v", f.store.Items[id])
	}
}

func TestRejectWithdrawAndComment(t *testing.T) {
	f := newFixture(t)
	id := f.pending()
	if _, err := f.manager.Reject(approvalstest.As(context.Background(), f.approver), id, "  ", 1); code(err) != string(approvals.CodeNoteRequired) {
		t.Fatalf("note required: %v", err)
	}
	a, err := f.manager.Reject(approvalstest.As(context.Background(), f.approver), id, "missing filter", 1)
	if err != nil || a.Status != approvals.StatusRejected || a.DecisionNote != "missing filter" {
		t.Fatalf("reject = %+v %v", a, err)
	}
	if _, err := f.manager.Withdraw(approvalstest.As(context.Background(), f.editor), id); code(err) != string(approvals.CodeAlreadyDecided) {
		t.Fatalf("withdraw decided: %v", err)
	}
	second := f.pending()
	if _, err := f.manager.Withdraw(approvalstest.As(context.Background(), f.approver), second); code(err) != string(approvals.CodeNotRequester) {
		t.Fatalf("withdraw by other: %v", err)
	}
	if a, err := f.manager.Withdraw(approvalstest.As(context.Background(), f.admin), second); err != nil || a.CancelReason != approvals.CancelWithdrawn {
		t.Fatalf("withdraw by workspace admin = %+v %v", a, err)
	}
	third := f.pending()
	if a, err := f.manager.Withdraw(approvalstest.As(context.Background(), f.editor), third); err != nil || a.Status != approvals.StatusCancelled {
		t.Fatalf("withdraw by requester = %+v %v", a, err)
	}
	if _, err := f.manager.Comment(approvalstest.As(context.Background(), f.viewer), third, ""); code(err) != string(approvals.CodeInvalidComment) {
		t.Fatalf("empty comment: %v", err)
	}
	if _, err := f.manager.Comment(approvalstest.As(context.Background(), f.viewer), third, strings.Repeat("x", 4001)); code(err) != string(approvals.CodeInvalidComment) {
		t.Fatalf("long comment: %v", err)
	}
	c, err := f.manager.Comment(approvalstest.As(context.Background(), f.viewer), third, "why prod?")
	if err != nil || c.Body != "why prod?" {
		t.Fatalf("comment = %+v %v", c, err)
	}
	last := f.notifier.Events()[len(f.notifier.Events())-1]
	if last.Kind != approvals.EventCommented || len(last.Recipients) != 1 || last.Recipients[0] != f.editor {
		t.Fatalf("comment event = %+v", last)
	}
}

func TestRequestSupersedesOlderPending(t *testing.T) {
	f := newFixture(t)
	old := f.pending()
	route := f.store.Items[old].RouteID
	newer := f.pending()
	f.store.Items[newer].RouteID = route
	a, err := f.manager.Request(approvalstest.As(context.Background(), f.editor), approvals.RequestInput{DeploymentID: newer, RouteID: route})
	if err != nil || a.ID != newer || !a.IsPending() {
		t.Fatalf("request = %+v %v", a, err)
	}
	if f.store.Items[old].Status != approvals.StatusCancelled || f.store.Items[old].CancelReason != approvals.CancelSuperseded {
		t.Fatalf("older = %+v", f.store.Items[old])
	}
	ev := f.notifier.Events()[0]
	if ev.Kind != approvals.EventRequested || len(ev.Recipients) != 2 {
		t.Fatalf("requested event = %+v", ev)
	}
}

func TestDetailAbilities(t *testing.T) {
	f := newFixture(t)
	id := f.pending()
	d, err := f.manager.Get(approvalstest.As(context.Background(), f.approver), id)
	if err != nil || !d.CanDecide || d.CanWithdraw || d.EligibleCount != 2 {
		t.Fatalf("approver detail = %+v %v", d, err)
	}
	d, _ = f.manager.Get(approvalstest.As(context.Background(), f.editor), id)
	if d.CanDecide || !d.CanWithdraw {
		t.Fatalf("requester detail = %+v", d)
	}
	d, _ = f.manager.Get(approvalstest.As(context.Background(), f.viewer), id)
	if d.CanDecide || d.CanWithdraw {
		t.Fatalf("viewer detail = %+v", d)
	}
}

func TestDestinationReaderReadsBranchConnections(t *testing.T) {
	raw := []byte(`{"version":1,"name":"r","source":{"connectionId":"x","topics":["a"]},"branches":[{"destination":{"connectionId":"` +
		prodConn.String() + `","topic":"o"}},{"destination":{"connectionId":"` + prodConn.String() + `","topic":"p"}}]}`)
	ids := approvals.DestinationReader{}.IDs(raw)
	if len(ids) > 1 {
		t.Fatalf("ids not distinct: %v", ids)
	}
	if len(approvals.DestinationReader{}.IDs([]byte(`nope`))) != 0 {
		t.Fatal("garbage config has destinations")
	}
}

// gateTable is testdata/approvals/gate_table.json (also read by the web DeployModeResolver test).
type gateTable struct {
	Cases []struct {
		Role    string `json:"role"`
		Prod    bool   `json:"prod"`
		Require bool   `json:"requireProdApproval"`
		Outcome string `json:"outcome"`
	} `json:"cases"`
}

// WorkspaceAdminRole names the workspace-admin row of the shared gate table.
const WorkspaceAdminRole = "workspace_admin"

func TestTeamProductionPolicyMatchesSharedGateTable(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "approvals", "gate_table.json"))
	if err != nil {
		t.Fatal(err)
	}
	var table gateTable
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatal(err)
	}
	matrix := rbac.DefaultRoleMatrix()
	for _, c := range table.Cases {
		perms := matrix.Permissions(rbac.Role(c.Role))
		if c.Role == WorkspaceAdminRole {
			perms = matrix.Permissions(rbac.RoleAdmin)
		}
		rights := approvals.CallerRights{DeployNonProd: perms.Has(rbac.PermRouteDeployNonProd), DeployProd: perms.Has(rbac.PermRouteDeployProd),
			Write: perms.Has(rbac.PermRouteWrite)}
		env := rbac.EnvDev
		if c.Prod {
			env = rbac.EnvProd
		}
		got := approvals.TeamProductionPolicy{}.Evaluate(context.Background(), rights,
			approvals.DeployTarget{Destinations: []rbac.Resource{{EnvTag: env}}}, approvals.TeamPolicy{RequireProdApproval: c.Require})
		if string(got.Outcome) != c.Outcome {
			t.Errorf("%+v: got %s", c, got.Outcome)
		}
	}
}
