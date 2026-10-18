package approvals_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/approvals"
	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/store/identity"
	"github.com/eventrouter/router/internal/testkit"
)

// itWorld is one migrated sandbox schema: a team, an editor (requester), two approvers, a viewer, a prod
// connection (sensitive) and a dev connection, one route with a version, plus the real Postgres stores.
type itWorld struct {
	sb                                  *testkit.PostgresSandbox
	team, route, version                uuid.UUID
	editor, approver, approver2, viewer uuid.UUID
	prodConn, devConn                   uuid.UUID
}

func newITWorld(t *testing.T) *itWorld {
	t.Helper()
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	w := &itWorld{sb: sb}
	w.team = w.scalar(t, `INSERT INTO teams (name) VALUES ('it-team') RETURNING id`)
	w.editor, w.approver = w.user(t, "editor@it", rbac.RoleEditor), w.user(t, "approver@it", rbac.RoleApprover)
	w.approver2, w.viewer = w.user(t, "approver2@it", rbac.RoleApprover), w.user(t, "viewer@it", rbac.RoleViewer)
	w.prodConn = w.scalar(t, `INSERT INTO connections (team_id, name, env_tag, bootstrap_servers, security_mode, sensitive)
		VALUES ($1, 'prod', 'prod', ARRAY['k:9092'], 'plaintext', true) RETURNING id`, w.team)
	w.devConn = w.scalar(t, `INSERT INTO connections (team_id, name, env_tag, bootstrap_servers, security_mode)
		VALUES ($1, 'dev', 'dev', ARRAY['k:9092'], 'plaintext') RETURNING id`, w.team)
	w.route = w.scalar(t, `INSERT INTO routes (team_id, name) VALUES ($1, 'orders') RETURNING id`, w.team)
	w.version = w.scalar(t, `INSERT INTO route_versions (route_id, version_no, config, config_hash)
		VALUES ($1, 1, jsonb_build_object('branches', jsonb_build_array(jsonb_build_object('destination', jsonb_build_object('connectionId', $2::text)))), 'h')
		RETURNING id`, w.route, w.prodConn)
	return w
}

func (w *itWorld) scalar(t *testing.T, sql string, args ...any) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := w.sb.DB.Pool().QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return id
}

func (w *itWorld) user(t *testing.T, email string, role rbac.Role) uuid.UUID {
	t.Helper()
	id := w.scalar(t, `INSERT INTO users (email, display_name, sso_subject) VALUES ($1, $1, $1) RETURNING id`, email)
	if _, err := w.sb.DB.Pool().Exec(context.Background(), `INSERT INTO role_bindings (user_id, team_id, role) VALUES ($1, $2, $3)`,
		id, w.team, string(role)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	return id
}

// pending inserts a pending_approval deployment requested by the editor.
func (w *itWorld) pending(t *testing.T) uuid.UUID {
	t.Helper()
	return w.scalar(t, `INSERT INTO deployments (route_id, version_id, status, requested_by) VALUES ($1, $2, 'pending_approval', $3) RETURNING id`,
		w.route, w.version, w.editor)
}

func (w *itWorld) status(t *testing.T, id uuid.UUID) (string, *string) {
	t.Helper()
	var status string
	var note *string
	if err := w.sb.DB.Pool().QueryRow(context.Background(), `SELECT status::text, decision_note FROM deployments WHERE id = $1`, id).Scan(&status, &note); err != nil {
		t.Fatal(err)
	}
	return status, note
}

// sqlActivator moves the deployment to rolling_out in the caller's transaction, or fails activation.
type sqlActivator struct {
	db   *store.DB
	fail error
}

func (a sqlActivator) Revalidate(context.Context, uuid.UUID) error { return nil }

func (a sqlActivator) Activate(ctx context.Context, id, approver uuid.UUID) error {
	if a.fail != nil {
		return a.fail
	}
	_, err := a.db.Q(ctx).Exec(ctx, `UPDATE deployments SET status = 'rolling_out', approved_by = $2, approved_at = now(), started_at = now()
		WHERE id = $1 AND status = 'pending_approval'`, id, approver)
	return err
}

func (w *itWorld) access() *rbac.DefaultManager {
	db := w.sb.DB
	return rbac.NewManager(rbac.ManagerDeps{Engine: rbac.NewRoleBindingEngine(rbac.DefaultRoleMatrix(), rbac.DefaultPermissionCatalog()),
		Subjects: rbac.NewSubjectResolver(rbac.NewTTLSubjectCache(0, nil), rbac.NewPostgresSubjectLoader(db)), Matrix: rbac.DefaultRoleMatrix(),
		Catalog: rbac.DefaultPermissionCatalog(), Conns: rbac.NewConnectionAccess(rbac.NewPostgresConnectionDirectory(db), rbac.NewPostgresGrantStore(db)),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

func (w *itWorld) manager(activator approvals.Activator) *approvals.DefaultManager {
	db := w.sb.DB
	st := approvals.NewPostgresStore(db)
	return approvals.NewManager(approvals.Deps{Tx: store.NewTxManager(db), Store: st, Access: w.access(), Teams: rbac.NewPostgresTeamStore(db),
		Destinations: approvals.DestinationReader{Connections: rbac.NewPostgresConnectionDirectory(db)}, Policy: approvals.TeamProductionPolicy{},
		Activator: activator, Notifier: &approvals.MemoryNotifier{}, Trail: rbac.NewAuditTrail(audit.NewPostgresRecorder(db)),
		Metrics: approvals.NewMetrics(nil, st), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

func asUser(id uuid.UUID) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.PrincipalUser, UserID: id})
}

func TestIntegrationConcurrentApproveVsRejectExactlyOneWins(t *testing.T) {
	w := newITWorld(t)
	m := w.manager(sqlActivator{db: w.sb.DB})
	for round := 0; round < 3; round++ {
		id := w.pending(t)
		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Add(2)
		go func() { defer wg.Done(); _, errs[0] = m.Approve(asUser(w.approver), id, "", 1) }()
		go func() { defer wg.Done(); _, errs[1] = m.Reject(asUser(w.approver2), id, "no", 1) }()
		wg.Wait()
		wins := 0
		for _, err := range errs {
			if err == nil {
				wins++
				continue
			}
			if d := problem.NewCatalog().Resolve(err); d.Code != string(approvals.CodeAlreadyDecided) && d.Status != 412 {
				t.Fatalf("loser error = %+v (%v)", d, err)
			}
		}
		if wins != 1 {
			t.Fatalf("round %d: %d winners (%v)", round, wins, errs)
		}
		if _, err := w.sb.DB.Pool().Exec(context.Background(), `UPDATE deployments SET status = 'superseded' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIntegrationSupersedeAndPartialUniqueIndex(t *testing.T) {
	w := newITWorld(t)
	m := w.manager(sqlActivator{db: w.sb.DB})
	old := w.pending(t)
	_, err := w.sb.DB.Pool().Exec(context.Background(), `INSERT INTO deployments (route_id, version_id, status) VALUES ($1, $2, 'pending_approval')`,
		w.route, w.version)
	if !errors.Is(w.sb.DB.Translate(err), store.ErrConflict) {
		t.Fatalf("second pending for the route must violate the partial unique index: %v", err)
	}
	cancelled, err := approvals.NewPostgresStore(w.sb.DB).CancelOtherPending(context.Background(), w.route, uuid.New())
	if err != nil || len(cancelled) != 1 || cancelled[0] != old {
		t.Fatalf("cancel others = %v %v", cancelled, err)
	}
	newer := w.pending(t)
	a, err := m.Request(asUser(w.editor), approvals.RequestInput{DeploymentID: newer, RouteID: w.route})
	if err != nil || !a.IsPending() || len(a.Destinations) != 1 || a.Destinations[0].EnvTag != rbac.EnvProd {
		t.Fatalf("request = %+v %v", a, err)
	}
	got, err := m.Get(asUser(w.approver), old)
	if err != nil || got.Approval.Status != approvals.StatusCancelled || got.Approval.CancelReason != approvals.CancelSuperseded {
		t.Fatalf("old = %+v %v", got.Approval, err)
	}
}

func TestIntegrationApproveRollsBackWhenRevalidationFails(t *testing.T) {
	w := newITWorld(t)
	id := w.pending(t)
	invalid := problem.NewError(problem.KindValidation, "connection deleted").At("/branches/0/destination/connectionId")
	if _, err := w.manager(sqlActivator{db: w.sb.DB, fail: invalid}).Approve(asUser(w.approver), id, "looks fine", 1); !errors.Is(err, invalid) {
		t.Fatalf("approve = %v", err)
	}
	if status, note := w.status(t, id); status != string(approvals.StatusPending) || note != nil {
		t.Fatalf("not rolled back: %s %v", status, note)
	}
	if _, err := w.manager(sqlActivator{db: w.sb.DB}).Approve(asUser(w.editor), id, "", 1); err == nil {
		t.Fatal("requester (editor) approved")
	}
	a, err := w.manager(sqlActivator{db: w.sb.DB}).Approve(asUser(w.approver), id, "ok", 1)
	if err != nil || a.Status != approvals.StatusRollingOut || a.DecidedBy == nil || a.DecidedBy.ID != w.approver || a.DecisionNote != "ok" {
		t.Fatalf("approve = %+v %v", a, err)
	}
	items, _, err := w.manager(nil).List(asUser(w.viewer), approvals.ListFilter{RouteID: &w.route}, approvals.Page{Limit: 10})
	if err != nil || len(items) != 1 {
		t.Fatalf("list = %d %v", len(items), err)
	}
}

func TestIntegrationGateAndSampleVisibility(t *testing.T) {
	w := newITWorld(t)
	m := w.manager(nil)
	prod := approvals.DeployTarget{RouteID: w.route, TeamID: w.team, Destinations: []rbac.Resource{{EnvTag: rbac.EnvProd}}}
	res, err := m.Gate(asUser(w.editor), prod)
	if err != nil || res.Outcome != approvals.OutcomeRequireApproval || res.Approvers != 2 {
		t.Fatalf("gate = %+v %v", res, err)
	}
	access := w.access()
	cases := []struct {
		who  uuid.UUID
		conn uuid.UUID
		want rbac.Visibility
	}{{w.viewer, w.devConn, rbac.VisibilityMasked}, {w.editor, w.devConn, rbac.VisibilityRaw}, {w.editor, w.prodConn, rbac.VisibilityMasked}}
	for _, c := range cases {
		got, err := access.SampleVisibility(asUser(c.who), rbac.Resource{Type: rbac.ResourceConnection, ID: c.conn.String()})
		if err != nil || got != c.want {
			t.Errorf("visibility %v on %v = %s %v", c.who, c.conn, got, err)
		}
	}
}

func TestIntegrationTeamsGrantsAndInvitations(t *testing.T) {
	w := newITWorld(t)
	db, ctx := w.sb.DB, context.Background()
	teams, trail := rbac.NewPostgresTeamStore(db), rbac.NewAuditTrail(audit.NewPostgresRecorder(db))
	svc := rbac.NewTeamService(rbac.TeamServiceDeps{Tx: store.NewTxManager(db), Teams: teams, Trail: trail, Defaults: rbac.DefaultTeamDefaults})
	owner, _ := teams.Get(ctx, w.team)
	if err := svc.Delete(ctx, w.team, owner.RowVersion); problem.NewCatalog().Resolve(err).Code != string(rbac.CodeTeamNotEmpty) {
		t.Fatalf("delete owner team: %v", err)
	}
	other, err := svc.Create(ctx, rbac.CreateTeam{Name: "it-other"})
	if err != nil || !other.RequireProdApproval {
		t.Fatalf("create = %+v %v", other, err)
	}
	access := w.access()
	if ok, _ := access.CanUseConnection(ctx, other.ID, w.prodConn); ok {
		t.Fatal("ungranted use allowed")
	}
	grants := rbac.NewGrantService(rbac.GrantServiceDeps{Tx: store.NewTxManager(db), Grants: rbac.NewPostgresGrantStore(db),
		Access: rbac.NewConnectionAccess(rbac.NewPostgresConnectionDirectory(db), rbac.NewPostgresGrantStore(db)), Teams: teams, Trail: trail})
	if _, err := grants.Add(ctx, other.ID, w.prodConn); err != nil {
		t.Fatal(err)
	}
	if ok, _ := access.CanUseConnection(ctx, other.ID, w.prodConn); !ok {
		t.Fatal("granted use denied")
	}
	if err := svc.Delete(ctx, other.ID, other.RowVersion); err != nil {
		t.Fatalf("delete empty team (grants cascade): %v", err)
	}
	w.checkInvitation(t, ctx)
}

// checkInvitation: a pending user is taken over by the first login with that email (trigger), once.
func (w *itWorld) checkInvitation(t *testing.T, ctx context.Context) {
	t.Helper()
	invites := rbac.NewPostgresInvitationStore(w.sb.DB)
	id, err := invites.InsertInvited(ctx, "new@it", "New", &w.approver)
	if err != nil {
		t.Fatal(err)
	}
	u, err := identity.New(w.sb.DB).Users.Upsert(ctx, auth.Identity{Subject: "oidc|new", Email: "NEW@it", DisplayName: "New Person"})
	if err != nil || u.ID != id {
		t.Fatalf("login did not claim the invitation: %+v %v (want %s)", u, err, id)
	}
	if ok, err := invites.Accept(ctx, id); err != nil || !ok {
		t.Fatalf("accept = %v %v", ok, err)
	}
	if ok, _ := invites.Accept(ctx, id); ok {
		t.Fatal("accepted twice")
	}
}
