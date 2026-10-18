package identity

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/store"
)

// RoleBindingRepository manages role_bindings rows auth needs.
type RoleBindingRepository struct {
	db *store.DB
}

// Ensure binds role on team unless the user already has a binding there; reports whether it inserted.
func (r *RoleBindingRepository) Ensure(ctx context.Context, userID, teamID uuid.UUID, role auth.Role) (bool, error) {
	tag, err := r.db.Q(ctx).Exec(ctx,
		`INSERT INTO role_bindings (user_id, team_id, role) VALUES ($1, $2, $3::team_role) ON CONFLICT (user_id, team_id) DO NOTHING`,
		userID, teamID, string(role))
	if err != nil {
		return false, r.db.Translate(err)
	}
	return tag.RowsAffected() > 0, nil
}

// Any reports whether any role binding exists in the workspace.
func (r *RoleBindingRepository) Any(ctx context.Context) (bool, error) {
	var exists bool
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM role_bindings)`).Scan(&exists)
	return exists, r.db.Translate(err)
}

// RolesFor returns the user's team roles ordered by team name.
func (r *RoleBindingRepository) RolesFor(ctx context.Context, userID uuid.UUID) ([]auth.TeamRole, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT t.id, t.name, rb.role::text FROM role_bindings rb
		JOIN teams t ON t.id = rb.team_id WHERE rb.user_id = $1 ORDER BY t.name`, userID)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	defer rows.Close()
	roles := []auth.TeamRole{}
	for rows.Next() {
		var tr auth.TeamRole
		var role string
		if err := rows.Scan(&tr.TeamID, &tr.TeamName, &role); err != nil {
			return nil, err
		}
		tr.Role = auth.Role(role)
		roles = append(roles, tr)
	}
	return roles, rows.Err()
}
