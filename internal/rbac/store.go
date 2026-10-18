package rbac

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// SubjectLoader reads a subject (admin flag, disabled state, bindings) from storage.
type SubjectLoader interface {
	Load(ctx context.Context, userID uuid.UUID) (Subject, error)
}

// RoleBinding is one user's role in one team.
type RoleBinding struct {
	UserID     uuid.UUID        `json:"userId"`
	TeamID     TeamID           `json:"teamId"`
	TeamName   string           `json:"teamName"`
	Role       Role             `json:"role"`
	CreatedAt  time.Time        `json:"createdAt"`
	RowVersion store.RowVersion `json:"rowVersion"`
}

// BindingFilter narrows binding lists; nil members do not filter.
type BindingFilter struct {
	UserID *uuid.UUID
	TeamID *TeamID
}

// BindingStore persists role bindings.
type BindingStore interface {
	List(ctx context.Context, f BindingFilter) ([]RoleBinding, error)
	GetForUpdate(ctx context.Context, userID uuid.UUID, team TeamID) (RoleBinding, error)
	Insert(ctx context.Context, userID uuid.UUID, team TeamID, role Role) (RoleBinding, error)
	UpdateRole(ctx context.Context, userID uuid.UUID, team TeamID, role Role) (RoleBinding, error)
	Delete(ctx context.Context, userID uuid.UUID, team TeamID) error
}

// AdminStore serialises admin changes and counts the remaining administrators.
type AdminStore interface {
	Lock(ctx context.Context) error
	ActiveWorkspaceAdmins(ctx context.Context, excluding uuid.UUID) (int, error)
	ActiveTeamAdmins(ctx context.Context, team TeamID, excluding uuid.UUID) (int, error)
	PromoteWorkspaceAdmin(ctx context.Context, userID uuid.UUID) (bool, error)
}

// LockID names transaction-scoped advisory locks owned by access control.
type LockID int64

const AdminChangeLock LockID = 7003

// PostgresSubjectLoader loads subjects from users + role_bindings.
type PostgresSubjectLoader struct {
	db *store.DB
}

// NewPostgresSubjectLoader builds the loader.
func NewPostgresSubjectLoader(db *store.DB) *PostgresSubjectLoader {
	return &PostgresSubjectLoader{db: db}
}

// Load implements SubjectLoader; a missing user is store.ErrNotFound.
func (l *PostgresSubjectLoader) Load(ctx context.Context, userID uuid.UUID) (Subject, error) {
	s := Subject{UserID: userID, Kind: SubjectUser, Bindings: map[TeamID]Role{}}
	err := l.db.Q(ctx).QueryRow(ctx, `SELECT is_workspace_admin, disabled_at IS NOT NULL FROM users WHERE id = $1`, userID).
		Scan(&s.WorkspaceAdmin, &s.Disabled)
	if err != nil {
		return Subject{}, l.db.Translate(err)
	}
	rows, err := l.db.Q(ctx).Query(ctx, `SELECT team_id, role::text FROM role_bindings WHERE user_id = $1`, userID)
	if err != nil {
		return Subject{}, l.db.Translate(err)
	}
	defer rows.Close()
	for rows.Next() {
		var team TeamID
		var role string
		if err := rows.Scan(&team, &role); err != nil {
			return Subject{}, err
		}
		s.Bindings[team] = Role(role)
	}
	return s, rows.Err()
}

// PostgresAdminStore implements AdminStore.
type PostgresAdminStore struct {
	db *store.DB
}

// NewPostgresAdminStore builds the store.
func NewPostgresAdminStore(db *store.DB) *PostgresAdminStore {
	return &PostgresAdminStore{db: db}
}

// Lock takes the admin-change advisory lock for the current transaction.
func (s *PostgresAdminStore) Lock(ctx context.Context) error {
	_, err := s.db.Q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(AdminChangeLock))
	return s.db.Translate(err)
}

// ActiveWorkspaceAdmins counts enabled workspace admins other than excluding.
func (s *PostgresAdminStore) ActiveWorkspaceAdmins(ctx context.Context, excluding uuid.UUID) (int, error) {
	var n int
	err := s.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM users
		WHERE is_workspace_admin AND disabled_at IS NULL AND id <> $1`, excluding).Scan(&n)
	return n, s.db.Translate(err)
}

// ActiveTeamAdmins counts enabled users other than excluding bound as admin on team.
func (s *PostgresAdminStore) ActiveTeamAdmins(ctx context.Context, team TeamID, excluding uuid.UUID) (int, error) {
	var n int
	err := s.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM role_bindings rb JOIN users u ON u.id = rb.user_id
		WHERE rb.team_id = $1 AND rb.role = 'admin' AND u.disabled_at IS NULL AND u.id <> $2`, team, excluding).Scan(&n)
	return n, s.db.Translate(err)
}

// PromoteWorkspaceAdmin sets the flag; it reports whether anything changed.
func (s *PostgresAdminStore) PromoteWorkspaceAdmin(ctx context.Context, userID uuid.UUID) (bool, error) {
	tag, err := s.db.Q(ctx).Exec(ctx, `UPDATE users SET is_workspace_admin = true WHERE id = $1 AND NOT is_workspace_admin`, userID)
	if err != nil {
		return false, s.db.Translate(err)
	}
	return tag.RowsAffected() > 0, nil
}
