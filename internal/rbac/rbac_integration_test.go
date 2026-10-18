package rbac_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/testkit"
)

// Integration timings: invalidation must arrive well before the (long) cache TTL would expire.
const (
	itCacheTTL        = time.Hour
	itNotifyDeadline  = 10 * time.Second
	itNotifyPoll      = 50 * time.Millisecond
	itListenerSettled = 300 * time.Millisecond
	itAuditEntries    = 7
	itAuditPage       = 3
)

// itWorld is one migrated sandbox schema with seeded users, a team and the real Postgres stores.
type itWorld struct {
	sb   *testkit.PostgresSandbox
	team uuid.UUID
}

func newITWorld(t *testing.T) *itWorld {
	t.Helper()
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	w := &itWorld{sb: sb}
	w.team = w.scalar(t, `INSERT INTO teams (name) VALUES ('it-team') RETURNING id`)
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

// addUser inserts a user bound to the world's team with role, optionally a workspace admin.
func (w *itWorld) addUser(t *testing.T, email string, role rbac.Role, workspaceAdmin bool) uuid.UUID {
	t.Helper()
	id := w.scalar(t, `INSERT INTO users (email, display_name, is_workspace_admin) VALUES ($1, $1, $2) RETURNING id`, email, workspaceAdmin)
	if _, err := w.sb.DB.Pool().Exec(context.Background(), `INSERT INTO role_bindings (user_id, team_id, role) VALUES ($1, $2, $3)`,
		id, w.team, string(role)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	return id
}

type noRevoke struct{}

func (noRevoke) RevokeAllForUser(context.Context, uuid.UUID) error { return nil }

func (w *itWorld) userService() *rbac.UserService {
	db := w.sb.DB
	return rbac.NewUserService(rbac.UserServiceDeps{Tx: store.NewTxManager(db), Users: rbac.NewPostgresUserStore(db),
		Guard: rbac.NewAdminGuard(rbac.NewPostgresAdminStore(db)), Sessions: noRevoke{}, Trail: rbac.NewAuditTrail(audit.NewPostgresRecorder(db))})
}

func (w *itWorld) manager(t *testing.T, ctx context.Context) *rbac.DefaultManager {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine, err := rbac.NewEngineRegistry().Build(config.RBACEngineRoleBinding, rbac.EngineDeps{Matrix: rbac.DefaultRoleMatrix(), Catalog: rbac.DefaultPermissionCatalog()})
	if err != nil {
		t.Fatal(err)
	}
	cache := rbac.NewTTLSubjectCache(itCacheTTL, time.Now)
	go rbac.NewInvalidationListener(w.sb.DB.Pool(), cache, log).Run(ctx)
	return rbac.NewManager(rbac.ManagerDeps{Engine: engine, Subjects: rbac.NewSubjectResolver(cache, rbac.NewPostgresSubjectLoader(w.sb.DB)),
		Matrix: rbac.DefaultRoleMatrix(), Catalog: rbac.DefaultPermissionCatalog(), Log: log})
}

func TestIntegrationSubjectLoader(t *testing.T) {
	w := newITWorld(t)
	admin := w.addUser(t, "admin@it", rbac.RoleAdmin, true)
	s, err := rbac.NewPostgresSubjectLoader(w.sb.DB).Load(context.Background(), admin)
	if err != nil || !s.WorkspaceAdmin || s.Disabled || s.Bindings[w.team] != rbac.RoleAdmin {
		t.Fatalf("subject: %+v %v", s, err)
	}
	if _, err := rbac.NewPostgresSubjectLoader(w.sb.DB).Load(context.Background(), uuid.New()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing user: %v", err)
	}
}

// Two admins demote each other at the same time: the advisory lock serialises the guard, so exactly one wins.
func TestIntegrationConcurrentLastAdminDemotion(t *testing.T) {
	w := newITWorld(t)
	users := []uuid.UUID{w.addUser(t, "a@it", rbac.RoleAdmin, true), w.addUser(t, "b@it", rbac.RoleAdmin, true)}
	svc, demote := w.userService(), false
	errs := make([]error, len(users))
	var wg sync.WaitGroup
	for i, id := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u, err := svc.Get(context.Background(), id)
			if err == nil {
				_, err = svc.Update(context.Background(), id, rbac.UserChange{WorkspaceAdmin: &demote}, u.RowVersion)
			}
			errs[i] = err
		}()
	}
	wg.Wait()
	lost := 0
	for _, err := range errs {
		if errors.Is(err, rbac.ErrLastAdmin) {
			lost++
		} else if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if lost != 1 {
		t.Fatalf("want exactly one last_admin rejection, got %v", errs)
	}
}

// A binding change NOTIFYs rbac_changed; both managers drop their cached subject long before the TTL.
func TestIntegrationNotifyInvalidatesTwoManagers(t *testing.T) {
	w := newITWorld(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	user := w.addUser(t, "editor@it", rbac.RoleEditor, false)
	managers := []*rbac.DefaultManager{w.manager(t, ctx), w.manager(t, ctx)}
	asUser := auth.WithPrincipal(ctx, auth.Principal{Kind: auth.PrincipalUser, UserID: user})
	target := rbac.Resource{Type: rbac.ResourceConnection, TeamID: w.team}
	for _, m := range managers {
		if m.Decide(asUser, rbac.PermConnectionWrite, target).Allowed {
			t.Fatal("editor must not write connections")
		}
	}
	time.Sleep(itListenerSettled)
	if _, err := w.sb.DB.Pool().Exec(ctx, `UPDATE role_bindings SET role = 'admin' WHERE user_id = $1`, user); err != nil {
		t.Fatal(err)
	}
	for i, m := range managers {
		deadline := time.Now().Add(itNotifyDeadline)
		for !m.Decide(asUser, rbac.PermConnectionWrite, target).Allowed {
			if time.Now().After(deadline) {
				t.Fatalf("manager %d still serves the cached editor subject", i)
			}
			time.Sleep(itNotifyPoll)
		}
	}
}

// Entries recorded through the real recorder page back newest first without gaps or duplicates.
func TestIntegrationAuditKeysetPaging(t *testing.T) {
	w := newITWorld(t)
	ctx := context.Background()
	recorder, catalog := audit.NewPostgresRecorder(w.sb.DB), audit.NewCatalog()
	recorder.AddHook(audit.NewCatalogHook(catalog))
	for i := 0; i < itAuditEntries; i++ {
		action := audit.ActionRouteCreated
		if i%2 == 0 {
			action = audit.ActionSampleViewedRaw
		}
		if err := recorder.Record(ctx, audit.Entry{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: action, Object: audit.ObjectRoute}); err != nil {
			t.Fatal(err)
		}
	}
	mgr := audit.NewManager(audit.NewPostgresReader(w.sb.DB), catalog)
	seen, page := map[int64]bool{}, audit.Page{Limit: itAuditPage}
	for {
		res, err := mgr.Query(ctx, audit.Filter{}, page)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range res.Items {
			if seen[e.ID] {
				t.Fatalf("duplicate entry %d", e.ID)
			}
			seen[e.ID] = true
		}
		if res.Next == nil {
			break
		}
		page.After = res.Next
	}
	if len(seen) != itAuditEntries {
		t.Fatalf("paged %d entries, want %d", len(seen), itAuditEntries)
	}
	sensitive, err := mgr.Query(ctx, audit.Filter{SensitiveOnly: true}, audit.Page{})
	if err != nil || len(sensitive.Items) != (itAuditEntries+1)/2 {
		t.Fatalf("sensitive filter: %d %v", len(sensitive.Items), err)
	}
	if _, err := w.sb.DB.Pool().Exec(ctx, `UPDATE audit_log SET action = 'x'`); err == nil {
		t.Fatal("audit_log must be append-only")
	}
}

// Entries without a stored label show the live object name (users, teams) and are searchable by it.
func TestIntegrationAuditLabelsResolveNames(t *testing.T) {
	w := newITWorld(t)
	ctx := context.Background()
	user := w.addUser(t, "named@it", rbac.RoleEditor, false)
	recorder := audit.NewPostgresRecorder(w.sb.DB)
	for _, e := range []audit.Entry{
		{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: audit.ActionUserUpdated, Object: audit.ObjectUser, ObjectID: user.String()},
		{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: audit.ActionTeamUpdated, Object: audit.ObjectTeam, ObjectID: w.team.String()},
	} {
		if err := recorder.Record(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	mgr := audit.NewManager(audit.NewPostgresReader(w.sb.DB), audit.NewCatalog())
	res, err := mgr.Query(ctx, audit.Filter{}, audit.Page{})
	if err != nil || len(res.Items) != 2 || res.Items[0].ObjectLabel != "it-team" || res.Items[1].ObjectLabel != "named@it" {
		t.Fatalf("labels not resolved: %+v %v", res.Items, err)
	}
	found, err := mgr.Query(ctx, audit.Filter{Text: "it-team"}, audit.Page{})
	if err != nil || len(found.Items) != 1 {
		t.Fatalf("search by resolved name: %d %v", len(found.Items), err)
	}
}
