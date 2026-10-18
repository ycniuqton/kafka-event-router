// Package rbactest provides in-memory access-control stores (subjects, bindings, admins, users, sessions,
// audit) and a pass-through TxManager, so services and handlers can be tested without Postgres.
package rbactest

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// PassTx runs units of work without a transaction.
type PassTx struct{}

// WithinTx implements store.TxManager.
func (PassTx) WithinTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

// BindingKey identifies a binding by (user, team).
type BindingKey [2]uuid.UUID

// World is one in-memory workspace implementing SubjectLoader, BindingStore, AdminStore, SessionRevoker and audit.Recorder.
type World struct {
	mu       sync.Mutex
	Users    map[uuid.UUID]*rbac.User
	Bindings map[BindingKey]rbac.RoleBinding
	Revoked  []uuid.UUID
	Entries  []audit.Entry
}

// NewWorld returns an empty workspace.
func NewWorld() *World {
	return &World{Users: map[uuid.UUID]*rbac.User{}, Bindings: map[BindingKey]rbac.RoleBinding{}}
}

// AddUser creates a user and returns its id.
func (w *World) AddUser(admin bool, status rbac.UserStatus) uuid.UUID {
	id := uuid.New()
	w.Users[id] = &rbac.User{ID: id, Email: id.String()[:8] + "@example.io", DisplayName: "user", WorkspaceAdmin: admin,
		Status: status, Teams: []rbac.UserTeam{}, RowVersion: 1}
	return id
}

// Bind sets a role binding at version 1.
func (w *World) Bind(user uuid.UUID, team rbac.TeamID, role rbac.Role) {
	w.Bindings[BindingKey{user, team}] = rbac.RoleBinding{UserID: user, TeamID: team, Role: role, RowVersion: 1}
}

// Load implements rbac.SubjectLoader.
func (w *World) Load(_ context.Context, id uuid.UUID) (rbac.Subject, error) {
	u, ok := w.Users[id]
	if !ok {
		return rbac.Subject{}, store.ErrNotFound
	}
	s := rbac.Subject{UserID: id, WorkspaceAdmin: u.WorkspaceAdmin, Disabled: u.Status == rbac.UserDisabled, Bindings: map[rbac.TeamID]rbac.Role{}}
	for k, b := range w.Bindings {
		if k[0] == id {
			s.Bindings[k[1]] = b.Role
		}
	}
	return s, nil
}

// List implements rbac.BindingStore.
func (w *World) List(_ context.Context, f rbac.BindingFilter) ([]rbac.RoleBinding, error) {
	out := []rbac.RoleBinding{}
	for _, b := range w.Bindings {
		if (f.UserID == nil || *f.UserID == b.UserID) && (f.TeamID == nil || *f.TeamID == b.TeamID) {
			out = append(out, b)
		}
	}
	return out, nil
}

// GetForUpdate implements rbac.BindingStore.
func (w *World) GetForUpdate(_ context.Context, u uuid.UUID, t rbac.TeamID) (rbac.RoleBinding, error) {
	b, ok := w.Bindings[BindingKey{u, t}]
	if !ok {
		return rbac.RoleBinding{}, store.ErrNotFound
	}
	return b, nil
}

// Insert implements rbac.BindingStore.
func (w *World) Insert(_ context.Context, u uuid.UUID, t rbac.TeamID, r rbac.Role) (rbac.RoleBinding, error) {
	w.Bind(u, t, r)
	return w.Bindings[BindingKey{u, t}], nil
}

// UpdateRole implements rbac.BindingStore.
func (w *World) UpdateRole(_ context.Context, u uuid.UUID, t rbac.TeamID, r rbac.Role) (rbac.RoleBinding, error) {
	b := w.Bindings[BindingKey{u, t}]
	b.Role, b.RowVersion = r, b.RowVersion+1
	w.Bindings[BindingKey{u, t}] = b
	return b, nil
}

// Delete implements rbac.BindingStore.
func (w *World) Delete(_ context.Context, u uuid.UUID, t rbac.TeamID) error {
	delete(w.Bindings, BindingKey{u, t})
	return nil
}

// Lock implements rbac.AdminStore.
func (w *World) Lock(context.Context) error { return nil }

// ActiveWorkspaceAdmins implements rbac.AdminStore.
func (w *World) ActiveWorkspaceAdmins(_ context.Context, excluding uuid.UUID) (int, error) {
	n := 0
	for id, u := range w.Users {
		if u.WorkspaceAdmin && u.Status != rbac.UserDisabled && id != excluding {
			n++
		}
	}
	return n, nil
}

// ActiveTeamAdmins implements rbac.AdminStore.
func (w *World) ActiveTeamAdmins(_ context.Context, team rbac.TeamID, excluding uuid.UUID) (int, error) {
	n := 0
	for k, b := range w.Bindings {
		if k[1] == team && b.Role == rbac.RoleAdmin && k[0] != excluding && w.Users[k[0]].Status != rbac.UserDisabled {
			n++
		}
	}
	return n, nil
}

// PromoteWorkspaceAdmin implements rbac.AdminStore.
func (w *World) PromoteWorkspaceAdmin(_ context.Context, id uuid.UUID) (bool, error) {
	u := w.Users[id]
	if u.WorkspaceAdmin {
		return false, nil
	}
	u.WorkspaceAdmin = true
	return true, nil
}

// RevokeAllForUser implements rbac.SessionRevoker.
func (w *World) RevokeAllForUser(_ context.Context, id uuid.UUID) error {
	w.Revoked = append(w.Revoked, id)
	return nil
}

// Record implements audit.Recorder.
func (w *World) Record(_ context.Context, e audit.Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Entries = append(w.Entries, e)
	return nil
}

// UserStore adapts a World to rbac.UserStore.
type UserStore struct{ W *World }

// List implements rbac.UserStore (every user, one page).
func (m UserStore) List(context.Context, rbac.UserFilter, rbac.Page) ([]rbac.User, *store.Cursor, error) {
	out := []rbac.User{}
	for _, u := range m.W.Users {
		out = append(out, *u)
	}
	return out, nil, nil
}

// Get implements rbac.UserStore.
func (m UserStore) Get(_ context.Context, id uuid.UUID) (rbac.User, error) {
	u, ok := m.W.Users[id]
	if !ok {
		return rbac.User{}, store.ErrNotFound
	}
	return *u, nil
}

// Update implements rbac.UserStore.
func (m UserStore) Update(_ context.Context, id uuid.UUID, c rbac.UserChange, expected store.RowVersion) error {
	u := m.W.Users[id]
	if u.RowVersion != expected {
		return &store.PreconditionFailedError{Current: u.RowVersion}
	}
	if c.DisplayName != nil {
		u.DisplayName = *c.DisplayName
	}
	if c.WorkspaceAdmin != nil {
		u.WorkspaceAdmin = *c.WorkspaceAdmin
	}
	if c.Disabled != nil {
		u.Status = rbac.UserStatusOf{}.Derive(*c.Disabled, false)
	}
	u.RowVersion++
	return nil
}

// Teams is a TeamEnsurer that always returns ID.
type Teams struct{ ID uuid.UUID }

// EnsureByName implements rbac.TeamEnsurer.
func (t Teams) EnsureByName(context.Context, string) (uuid.UUID, error) { return t.ID, nil }

// Ensurer adapts a World to rbac.BindingEnsurer.
type Ensurer struct{ W *World }

// Ensure implements rbac.BindingEnsurer.
func (e Ensurer) Ensure(_ context.Context, u, t uuid.UUID, r auth.Role) (bool, error) {
	if _, ok := e.W.Bindings[BindingKey{u, t}]; ok {
		return false, nil
	}
	e.W.Bind(u, t, r)
	return true, nil
}
