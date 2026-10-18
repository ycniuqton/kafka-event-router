package rbac

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/logging"
)

// Manager is the only entry point business code uses for access decisions.
type Manager interface {
	Authorize(ctx context.Context, p Permission, r Resource) error
	Decide(ctx context.Context, p Permission, r Resource) Decision
	Effective(ctx context.Context, userID uuid.UUID) (Access, error)
	SampleVisibility(ctx context.Context, conn Resource) (Visibility, error)
	CanUseConnection(ctx context.Context, team TeamID, connID uuid.UUID) (bool, error)
}

// SubjectResolver loads subjects through the cache.
type SubjectResolver struct {
	cache  SubjectCache
	loader SubjectLoader
}

// NewSubjectResolver builds the resolver.
func NewSubjectResolver(cache SubjectCache, loader SubjectLoader) *SubjectResolver {
	return &SubjectResolver{cache: cache, loader: loader}
}

// Resolve returns the cached subject or loads and caches it.
func (r *SubjectResolver) Resolve(ctx context.Context, userID uuid.UUID) (Subject, error) {
	if s, ok := r.cache.Get(userID); ok {
		return s, nil
	}
	s, err := r.loader.Load(ctx, userID)
	if err != nil {
		return Subject{}, err
	}
	r.cache.Put(s)
	return s, nil
}

// DefaultManager decides with the configured PolicyEngine over cached subjects; it fails closed.
type DefaultManager struct {
	engine   PolicyEngine
	subjects *SubjectResolver
	messages ForbiddenMessages
	catalog  PermissionCatalog
	conns    *ConnectionAccess
	log      *slog.Logger
}

// ManagerDeps groups the collaborators of a DefaultManager.
type ManagerDeps struct {
	Engine   PolicyEngine
	Subjects *SubjectResolver
	Matrix   RoleMatrix
	Catalog  PermissionCatalog
	Conns    *ConnectionAccess
	Log      *slog.Logger
}

// NewManager builds the manager; without Conns, connection use is denied and visibility trusts the caller's resource.
func NewManager(d ManagerDeps) *DefaultManager {
	return &DefaultManager{engine: d.Engine, subjects: d.Subjects, messages: NewForbiddenMessages(d.Matrix), catalog: d.Catalog,
		conns: d.Conns, log: d.Log}
}

// Authorize returns nil, auth.ErrUnauthenticated, or a ForbiddenError naming the missing permission.
func (m *DefaultManager) Authorize(ctx context.Context, p Permission, r Resource) error {
	if !auth.PrincipalFrom(ctx).IsAuthenticated() {
		return auth.ErrUnauthenticated
	}
	d := m.Decide(ctx, p, r)
	if d.Allowed {
		return nil
	}
	return m.messages.Error(d)
}

// Decide evaluates p on r for the principal in ctx; workloads (internal listener) are always allowed.
func (m *DefaultManager) Decide(ctx context.Context, p Permission, r Resource) Decision {
	principal := auth.PrincipalFrom(ctx)
	if principal.Kind == auth.PrincipalWorkload {
		return Allow()
	}
	if principal.UserID == uuid.Nil {
		return Deny(ReasonNoBinding, p)
	}
	s, err := m.subjects.Resolve(ctx, principal.UserID)
	if err != nil {
		m.log.Error("rbac subject load failed", string(logging.AttrError), err)
		return Deny(ReasonEngineError, p)
	}
	return m.engine.Evaluate(ctx, s, p, r)
}

// Effective returns the caller's permission summary for /auth/me.
func (m *DefaultManager) Effective(ctx context.Context, userID uuid.UUID) (Access, error) {
	s, err := m.subjects.Resolve(ctx, userID)
	if err != nil {
		return Access{}, err
	}
	a := Access{WorkspaceAdmin: s.WorkspaceAdmin && !s.Disabled, Workspace: []Permission{}, Teams: map[string]TeamAccess{}}
	if a.WorkspaceAdmin {
		a.Workspace = m.catalog.InScope(ScopeWorkspace).Sorted()
	}
	for team, role := range s.Bindings {
		a.Teams[team.String()] = TeamAccess{Role: role, Permissions: m.engine.Permissions(s, team).Sorted()}
	}
	return a, nil
}

// SampleVisibility (flow 5.7): sensitive connections are masked for everyone; otherwise raw needs
// sample.read_raw on the connection's team (Viewers never have it). Unknown connections are masked with ErrUnknownConnection.
func (m *DefaultManager) SampleVisibility(ctx context.Context, conn Resource) (Visibility, error) {
	target, err := m.connectionResource(ctx, conn)
	if err != nil {
		return VisibilityMasked, err
	}
	if target.Sensitive {
		return VisibilityMasked, nil
	}
	if m.Decide(ctx, PermSampleReadRaw, target).Allowed {
		return VisibilityRaw, nil
	}
	return VisibilityMasked, nil
}

// connectionResource completes conn with the stored owner team and sensitivity; ids missing from the
// directory (or not ids at all) are ErrUnknownConnection.
func (m *DefaultManager) connectionResource(ctx context.Context, conn Resource) (Resource, error) {
	if m.conns == nil {
		return conn, nil
	}
	id, err := uuid.Parse(conn.ID)
	if err != nil {
		return conn, ErrUnknownConnection
	}
	info, ok, err := m.conns.Lookup(ctx, id)
	if err != nil {
		return conn, err
	}
	if !ok {
		return conn, ErrUnknownConnection
	}
	target := info.Resource()
	target.Sensitive = target.Sensitive || conn.Sensitive
	return target, nil
}

// CanUseConnection reports whether team owns connID or holds a grant on it (fails closed without a directory).
func (m *DefaultManager) CanUseConnection(ctx context.Context, team TeamID, connID uuid.UUID) (bool, error) {
	if m.conns == nil {
		return false, nil
	}
	return m.conns.CanUse(ctx, team, connID)
}
