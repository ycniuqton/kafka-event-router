package rbac

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// PostgresBindingStore implements BindingStore on role_bindings (joined with teams for names).
type PostgresBindingStore struct {
	db *store.DB
}

// NewPostgresBindingStore builds the store.
func NewPostgresBindingStore(db *store.DB) *PostgresBindingStore {
	return &PostgresBindingStore{db: db}
}

// selectBindingsSQL is the projection shared by binding reads.
const selectBindingsSQL = `SELECT rb.user_id, rb.team_id, t.name, rb.role::text, rb.created_at, rb.row_version
	FROM role_bindings rb JOIN teams t ON t.id = rb.team_id`

// List returns bindings ordered by team name then user.
func (s *PostgresBindingStore) List(ctx context.Context, f BindingFilter) ([]RoleBinding, error) {
	c := &bindingClauses{}
	c.add(f.UserID != nil, "rb.user_id", func() any { return *f.UserID })
	c.add(f.TeamID != nil, "rb.team_id", func() any { return *f.TeamID })
	rows, err := s.db.Q(ctx).Query(ctx, selectBindingsSQL+c.where()+` ORDER BY t.name, rb.user_id`, c.args...)
	if err != nil {
		return nil, s.db.Translate(err)
	}
	defer rows.Close()
	out := []RoleBinding{}
	for rows.Next() {
		b, err := s.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetForUpdate locks and returns one binding (store.ErrNotFound when absent).
func (s *PostgresBindingStore) GetForUpdate(ctx context.Context, userID uuid.UUID, team TeamID) (RoleBinding, error) {
	row := s.db.Q(ctx).QueryRow(ctx, selectBindingsSQL+` WHERE rb.user_id = $1 AND rb.team_id = $2 FOR UPDATE OF rb`, userID, team)
	b, err := s.scan(row)
	return b, s.db.Translate(err)
}

// Insert creates a binding.
func (s *PostgresBindingStore) Insert(ctx context.Context, userID uuid.UUID, team TeamID, role Role) (RoleBinding, error) {
	_, err := s.db.Q(ctx).Exec(ctx, `INSERT INTO role_bindings (user_id, team_id, role) VALUES ($1, $2, $3::team_role)`,
		userID, team, string(role))
	if err != nil {
		return RoleBinding{}, s.db.Translate(err)
	}
	return s.GetForUpdate(ctx, userID, team)
}

// UpdateRole changes the role of an existing binding (row_version bumps by trigger).
func (s *PostgresBindingStore) UpdateRole(ctx context.Context, userID uuid.UUID, team TeamID, role Role) (RoleBinding, error) {
	_, err := s.db.Q(ctx).Exec(ctx, `UPDATE role_bindings SET role = $3::team_role WHERE user_id = $1 AND team_id = $2`,
		userID, team, string(role))
	if err != nil {
		return RoleBinding{}, s.db.Translate(err)
	}
	return s.GetForUpdate(ctx, userID, team)
}

// Delete removes a binding.
func (s *PostgresBindingStore) Delete(ctx context.Context, userID uuid.UUID, team TeamID) error {
	_, err := s.db.Q(ctx).Exec(ctx, `DELETE FROM role_bindings WHERE user_id = $1 AND team_id = $2`, userID, team)
	return s.db.Translate(err)
}

func (s *PostgresBindingStore) scan(row interface{ Scan(...any) error }) (RoleBinding, error) {
	var b RoleBinding
	var role string
	var version int64
	err := row.Scan(&b.UserID, &b.TeamID, &b.TeamName, &role, &b.CreatedAt, &version)
	b.Role, b.RowVersion = Role(role), store.RowVersion(version)
	return b, err
}

// bindingClauses accumulates equality predicates with positional arguments.
type bindingClauses struct {
	parts []string
	args  []any
}

func (c *bindingClauses) add(cond bool, column string, arg func() any) {
	if !cond {
		return
	}
	c.args = append(c.args, arg())
	c.parts = append(c.parts, fmt.Sprintf("%s = $%d", column, len(c.args)))
}

func (c *bindingClauses) where() string {
	if len(c.parts) == 0 {
		return ""
	}
	out := " WHERE " + c.parts[0]
	for _, p := range c.parts[1:] {
		out += " AND " + p
	}
	return out
}
