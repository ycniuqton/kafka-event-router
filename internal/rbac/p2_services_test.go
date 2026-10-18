package rbac_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/rbac/rbactest"
	"github.com/eventrouter/router/internal/store"
)

var (
	connA = uuid.MustParse("00000000-0000-0000-0000-0000000000a1")
	connS = uuid.MustParse("00000000-0000-0000-0000-0000000000a2")
	connB = uuid.MustParse("00000000-0000-0000-0000-0000000000b1")
)

func directory() rbactest.Connections {
	return rbactest.Connections{
		connA: {ID: connA, TeamID: teamA, Name: "a", EnvTag: rbac.EnvDev},
		connS: {ID: connS, TeamID: teamA, Name: "secret", EnvTag: rbac.EnvProd, Sensitive: true},
		connB: {ID: connB, TeamID: teamB, Name: "b", EnvTag: rbac.EnvProd},
	}
}

func managerWithConnections(w *rbactest.World, grants *rbactest.Grants) *rbac.DefaultManager {
	return rbac.NewManager(rbac.ManagerDeps{Engine: rbac.NewRoleBindingEngine(rbac.DefaultRoleMatrix(), rbac.DefaultPermissionCatalog()),
		Subjects: rbac.NewSubjectResolver(rbac.NewTTLSubjectCache(0, nil), w), Matrix: rbac.DefaultRoleMatrix(), Catalog: rbac.DefaultPermissionCatalog(),
		Conns: rbac.NewConnectionAccess(grants.Connections, grants), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

func TestSampleVisibilityMatrix(t *testing.T) {
	w := rbactest.NewWorld()
	ids := map[rbac.Role]uuid.UUID{}
	for _, r := range (rbac.Roles{}).Ordered() {
		ids[r] = w.AddUser(false, rbac.UserActive)
		w.Bind(ids[r], teamA, r)
	}
	wsAdmin := w.AddUser(true, rbac.UserActive)
	m := managerWithConnections(w, rbactest.NewGrants(directory()))
	cases := []struct {
		who  uuid.UUID
		conn uuid.UUID
		want rbac.Visibility
	}{
		{ids[rbac.RoleViewer], connA, rbac.VisibilityMasked},
		{ids[rbac.RoleEditor], connA, rbac.VisibilityRaw},
		{ids[rbac.RoleApprover], connA, rbac.VisibilityRaw},
		{ids[rbac.RoleAdmin], connA, rbac.VisibilityRaw},
		{ids[rbac.RoleViewer], connS, rbac.VisibilityMasked},
		{ids[rbac.RoleEditor], connS, rbac.VisibilityMasked},
		{ids[rbac.RoleAdmin], connS, rbac.VisibilityMasked},
		{wsAdmin, connS, rbac.VisibilityMasked},
		{wsAdmin, connB, rbac.VisibilityRaw},
		{ids[rbac.RoleEditor], connB, rbac.VisibilityMasked},
	}
	for i, c := range cases {
		got, err := m.SampleVisibility(as(c.who), rbac.Resource{Type: rbac.ResourceConnection, ID: c.conn.String()})
		if err != nil || got != c.want {
			t.Errorf("case %d: %s %v", i, got, err)
		}
	}
}

func TestGrantServiceAndCanUseConnection(t *testing.T) {
	w := rbactest.NewWorld()
	grants := rbactest.NewGrants(directory())
	teams := rbactest.NewTeamStore()
	teams.Items[teamA], teams.Items[teamB] = rbac.Team{ID: teamA}, rbac.Team{ID: teamB}
	m := managerWithConnections(w, grants)
	svc := rbac.NewGrantService(rbac.GrantServiceDeps{Tx: rbactest.PassTx{}, Grants: grants, Access: rbac.NewConnectionAccess(grants.Connections, grants),
		Teams: teams, Trail: rbac.NewAuditTrail(w)})
	ctx := context.Background()
	if ok, _ := m.CanUseConnection(ctx, teamA, connA); !ok {
		t.Fatal("owner cannot use own connection")
	}
	if ok, _ := m.CanUseConnection(ctx, teamA, connB); ok {
		t.Fatal("ungranted cross-team use allowed")
	}
	if _, err := svc.Add(ctx, teamA, connA); !errors.Is(err, rbac.ErrGrantToOwner) {
		t.Fatalf("grant to owner: %v", err)
	}
	if _, err := svc.Add(ctx, teamA, uuid.New()); !errors.Is(err, rbac.ErrUnknownConnection) {
		t.Fatalf("unknown connection: %v", err)
	}
	if _, err := svc.Add(ctx, uuid.New(), connB); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown team: %v", err)
	}
	g, err := svc.Add(ctx, teamA, connB)
	if err != nil || g.ConnectionName != "b" {
		t.Fatalf("add = %+v %v", g, err)
	}
	if _, err := svc.Add(ctx, teamA, connB); err != nil || len(w.Entries) != 1 {
		t.Fatalf("repeat add must be a silent no-op: %v entries=%d", err, len(w.Entries))
	}
	if ok, _ := m.CanUseConnection(ctx, teamA, connB); !ok {
		t.Fatal("granted connection not usable")
	}
	if ok, _ := rbac.NewGrantChecker(m).CanUseConnection(ctx, teamA.String(), "garbage"); ok {
		t.Fatal("malformed id usable")
	}
	if err := svc.Remove(ctx, teamA, connB); err != nil || w.Entries[1].Action != audit.ActionGrantRemoved {
		t.Fatalf("remove: %v", err)
	}
	if err := svc.Remove(ctx, teamA, connB); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("remove twice: %v", err)
	}
	if ok, _ := rbac.NewManager(rbac.ManagerDeps{}).CanUseConnection(ctx, teamA, connA); ok {
		t.Fatal("manager without directory must fail closed")
	}
}

func TestTeamServiceRules(t *testing.T) {
	w := rbactest.NewWorld()
	teams := rbactest.NewTeamStore()
	svc := rbac.NewTeamService(rbac.TeamServiceDeps{Tx: rbactest.PassTx{}, Teams: teams, Trail: rbac.NewAuditTrail(w), Defaults: rbac.DefaultTeamDefaults})
	ctx := context.Background()
	payments, err := svc.Create(ctx, rbac.CreateTeam{Name: "  Payments "})
	if err != nil || payments.Name != "Payments" || !payments.RequireProdApproval {
		t.Fatalf("create = %+v %v", payments, err)
	}
	if _, err := svc.Create(ctx, rbac.CreateTeam{Name: "payments"}); !errors.Is(err, rbac.ErrTeamNameTaken) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := svc.Create(ctx, rbac.CreateTeam{Name: ""}); !errors.Is(err, rbac.ErrInvalidTeamName) {
		t.Fatalf("empty: %v", err)
	}
	off := false
	updated, err := svc.Update(ctx, payments.ID, rbac.TeamChange{RequireProdApproval: &off}, payments.RowVersion)
	if err != nil || updated.RequireProdApproval {
		t.Fatalf("update = %+v %v", updated, err)
	}
	var stale *store.PreconditionFailedError
	if _, err := svc.Update(ctx, payments.ID, rbac.TeamChange{RequireProdApproval: &off}, payments.RowVersion); !errors.As(err, &stale) {
		t.Fatalf("stale: %v", err)
	}
	teams.Usages[payments.ID] = rbac.TeamUsage{Routes: 2}
	err = svc.Delete(ctx, payments.ID, updated.RowVersion)
	if d := problem.NewCatalog().Resolve(err); d.Status != 409 || d.Code != string(rbac.CodeTeamNotEmpty) {
		t.Fatalf("delete non-empty = %+v", d)
	}
	teams.Usages[payments.ID] = rbac.TeamUsage{}
	if err := svc.Delete(ctx, payments.ID, updated.RowVersion); err != nil || len(teams.Items) != 0 {
		t.Fatalf("delete: %v", err)
	}
	actions := []audit.Action{}
	for _, e := range w.Entries {
		actions = append(actions, e.Action)
	}
	if len(actions) != 3 || actions[2] != audit.ActionTeamDeleted {
		t.Fatalf("audit = %v", actions)
	}
}

func TestEmailAddressNormalise(t *testing.T) {
	cases := map[string]bool{"Ana@Example.com": true, " bob@x.io ": true, "nope": false, "Ana <ana@x.io>": false, "": false}
	for in, ok := range cases {
		got, err := rbac.EmailAddress{}.Normalise(in)
		if (err == nil) != ok {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	if n := (rbac.EmailAddress{}).DisplayNameOf("ana.lee@x.io", ""); n != "ana.lee" {
		t.Errorf("display name = %q", n)
	}
}

func TestSampleAccessDecisions(t *testing.T) {
	w := rbactest.NewWorld()
	viewer, editor := w.AddUser(false, rbac.UserActive), w.AddUser(false, rbac.UserActive)
	w.Bind(viewer, teamA, rbac.RoleViewer)
	w.Bind(editor, teamA, rbac.RoleEditor)
	samples := rbac.NewSampleAccess(managerWithConnections(w, rbactest.NewGrants(directory())))
	cases := []struct {
		who     uuid.UUID
		surface rbac.SampleSurface
		conn    string
		masked  bool
		action  audit.Action
	}{
		{viewer, rbac.SurfacePreview, connA.String(), true, audit.ActionSampleViewed},
		{editor, rbac.SurfacePreview, connA.String(), false, audit.ActionSampleViewedRaw},
		{editor, rbac.SurfacePreview, connS.String(), true, audit.ActionSampleViewed},
		{editor, rbac.SurfacePreview, "", false, audit.ActionSampleViewedRaw},
		{viewer, rbac.SurfacePreview, "", true, audit.ActionSampleViewed},
		{viewer, rbac.SurfaceDLQ, connA.String(), true, audit.ActionDLQViewed},
		{editor, rbac.SurfaceLiveTail, connA.String(), false, audit.ActionLiveTailOpened},
	}
	for i, c := range cases {
		d, err := samples.Decide(as(c.who), c.surface, c.conn)
		if err != nil || d.Masked != c.masked || d.Action != c.action {
			t.Errorf("case %d: %+v %v", i, d, err)
		}
	}
}

func TestSampleAccessUnknownConnectionIsNotFound(t *testing.T) {
	w := rbactest.NewWorld()
	editor := w.AddUser(false, rbac.UserActive)
	w.Bind(editor, teamA, rbac.RoleEditor)
	samples := rbac.NewSampleAccess(managerWithConnections(w, rbactest.NewGrants(directory())))
	for _, conn := range []string{uuid.New().String(), "not-a-connection"} {
		d, err := samples.Decide(as(editor), rbac.SurfacePreview, conn)
		if !errors.Is(err, rbac.ErrUnknownConnection) || !d.Masked {
			t.Errorf("%s: %+v %v", conn, d, err)
		}
	}
}
