package rbac_test

import (
	"context"
	"errors"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/rbac/rbactest"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/problem"
)

func manager(w *rbactest.World) (*rbac.DefaultManager, *rbac.TTLSubjectCache) {
	cache := rbac.NewTTLSubjectCache(time.Minute, nil)
	m := rbac.NewManager(rbac.ManagerDeps{Engine: rbac.NewRoleBindingEngine(rbac.DefaultRoleMatrix(), rbac.DefaultPermissionCatalog()),
		Subjects: rbac.NewSubjectResolver(cache, w), Matrix: rbac.DefaultRoleMatrix(), Catalog: rbac.DefaultPermissionCatalog(),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	return m, cache
}

func as(id uuid.UUID) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.PrincipalUser, UserID: id})
}

func TestManagerAuthorize(t *testing.T) {
	w := rbactest.NewWorld()
	viewer := w.AddUser(false, rbac.UserActive)
	w.Bind(viewer, teamA, rbac.RoleViewer)
	m, _ := manager(w)

	if err := m.Authorize(context.Background(), rbac.PermRouteRead, rbac.Resource{}); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("anonymous: %v", err)
	}
	err := m.Authorize(as(viewer), rbac.PermRouteWrite, rbac.Resource{TeamID: teamA})
	var fe *rbac.ForbiddenError
	if !errors.As(err, &fe) || fe.MissingPermission() != string(rbac.PermRouteWrite) || fe.Detail == "" {
		t.Fatalf("viewer write: %v", err)
	}
	d := problem.NewCatalog().Resolve(err)
	if d.Status != 403 || d.MissingPermission != "route.write" || d.Code != "missing_permission" {
		t.Fatalf("problem = %+v", d)
	}
	if err := m.Authorize(as(viewer), rbac.PermRouteRead, rbac.Resource{TeamID: teamB}); err != nil {
		t.Fatalf("cross-team read: %v", err)
	}
	if err := m.Authorize(as(uuid.New()), rbac.PermRouteRead, rbac.Resource{}); !errors.As(err, &fe) || fe.Decision.Reason != rbac.ReasonEngineError {
		t.Fatalf("unknown subject fails closed: %v", err)
	}
	workload := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.PrincipalWorkload})
	if err := m.Authorize(workload, rbac.PermAuditRead, rbac.Resource{}); err != nil {
		t.Fatalf("workload: %v", err)
	}
}

func TestManagerCacheAndInvalidation(t *testing.T) {
	w := rbactest.NewWorld()
	u := w.AddUser(false, rbac.UserActive)
	w.Bind(u, teamA, rbac.RoleViewer)
	m, cache := manager(w)
	if m.Decide(as(u), rbac.PermRouteWrite, rbac.Resource{TeamID: teamA}).Allowed {
		t.Fatal("viewer allowed")
	}
	w.Bind(u, teamA, rbac.RoleEditor)
	if m.Decide(as(u), rbac.PermRouteWrite, rbac.Resource{TeamID: teamA}).Allowed {
		t.Fatal("cached subject must be used until invalidated")
	}
	l := rbac.NewInvalidationListener(nil, cache, nil)
	l.Apply(`{"user_id":"` + u.String() + `"}`)
	if !m.Decide(as(u), rbac.PermRouteWrite, rbac.Resource{TeamID: teamA}).Allowed {
		t.Fatal("invalidation must reload")
	}
	w.Bind(u, teamA, rbac.RoleViewer)
	l.Apply("garbage")
	if m.Decide(as(u), rbac.PermRouteWrite, rbac.Resource{TeamID: teamA}).Allowed {
		t.Fatal("unparsable payload must invalidate all")
	}
}

func TestManagerEffectiveAndVisibility(t *testing.T) {
	w := rbactest.NewWorld()
	editor := w.AddUser(false, rbac.UserActive)
	w.Bind(editor, teamA, rbac.RoleEditor)
	viewer := w.AddUser(false, rbac.UserActive)
	w.Bind(viewer, teamA, rbac.RoleViewer)
	admin := w.AddUser(true, rbac.UserActive)
	m, _ := manager(w)

	a, err := m.Effective(context.Background(), editor)
	if err != nil || a.WorkspaceAdmin || len(a.Workspace) != 0 || a.Teams[teamA.String()].Role != rbac.RoleEditor {
		t.Fatalf("editor access = %+v %v", a, err)
	}
	a, _ = m.Effective(context.Background(), admin)
	if !a.WorkspaceAdmin || len(a.Workspace) != 6 {
		t.Fatalf("admin access = %+v", a)
	}
	conn := rbac.Resource{Type: rbac.ResourceConnection, TeamID: teamA}
	cases := []struct {
		who       uuid.UUID
		sensitive bool
		want      rbac.Visibility
	}{
		{editor, false, rbac.VisibilityRaw}, {editor, true, rbac.VisibilityMasked}, {viewer, false, rbac.VisibilityMasked}, {admin, true, rbac.VisibilityMasked},
	}
	for _, c := range cases {
		conn.Sensitive = c.sensitive
		if got, _ := m.SampleVisibility(as(c.who), conn); got != c.want {
			t.Errorf("visibility %v sensitive=%v: %s", c.who, c.sensitive, got)
		}
	}
}

func TestTTLCacheExpiry(t *testing.T) {
	now := time.Unix(0, 0)
	c := rbac.NewTTLSubjectCache(30*time.Second, func() time.Time { return now })
	id := uuid.New()
	c.Put(rbac.Subject{UserID: id})
	if _, ok := c.Get(id); !ok {
		t.Fatal("fresh entry missing")
	}
	now = now.Add(31 * time.Second)
	if _, ok := c.Get(id); ok {
		t.Fatal("expired entry served")
	}
}
