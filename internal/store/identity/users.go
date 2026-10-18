package identity

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/store"
)

// UserRepository provisions users and loads principals; it implements auth.UserProvisioner,
// auth.PrincipalSource and auth.AdminBootstrapper.
type UserRepository struct {
	db       *store.DB
	teams    *TeamRepository
	bindings *RoleBindingRepository
}

// upsertUserSQL updates email/display name only when they changed (avoids row_version churn).
const upsertUserSQL = `INSERT INTO users (email, display_name, sso_subject) VALUES ($1, $2, $3)
	ON CONFLICT (sso_subject) DO UPDATE SET email = EXCLUDED.email, display_name = EXCLUDED.display_name
	WHERE users.email IS DISTINCT FROM EXCLUDED.email OR users.display_name IS DISTINCT FROM EXCLUDED.display_name
	RETURNING id, email, display_name, disabled_at IS NOT NULL`

// Upsert creates or refreshes the user identified by the identity's subject.
func (r *UserRepository) Upsert(ctx context.Context, id auth.Identity) (auth.User, error) {
	var u auth.User
	err := r.db.Q(ctx).QueryRow(ctx, upsertUserSQL, id.Email, id.DisplayName, id.Subject).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Disabled)
	if errors.Is(r.db.Translate(err), store.ErrNotFound) {
		return r.bySubject(ctx, id.Subject)
	}
	if err != nil {
		return auth.User{}, r.db.Translate(err)
	}
	return u, nil
}

// LoadUser returns the user and their team roles.
func (r *UserRepository) LoadUser(ctx context.Context, id uuid.UUID) (auth.User, []auth.TeamRole, error) {
	u := auth.User{ID: id}
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT email, display_name, disabled_at IS NOT NULL FROM users WHERE id = $1`, id).
		Scan(&u.Email, &u.DisplayName, &u.Disabled)
	if err != nil {
		return auth.User{}, nil, r.db.Translate(err)
	}
	roles, err := r.bindings.RolesFor(ctx, id)
	return u, roles, err
}

// MarkLogin records the user's latest sign-in (users.last_login_at; does not bump row_version).
func (r *UserRepository) MarkLogin(ctx context.Context, userID uuid.UUID, at time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE users SET last_login_at = $2 WHERE id = $1`, userID, at)
	return r.db.Translate(err)
}

// BindFirstAdmin makes u admin of teamName when no role binding exists yet (must run inside a tx).
func (r *UserRepository) BindFirstAdmin(ctx context.Context, u auth.User, teamName string) (bool, error) {
	if _, err := r.db.Q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(BootstrapAdminLock)); err != nil {
		return false, err
	}
	exists, err := r.bindings.Any(ctx)
	if err != nil || exists {
		return false, err
	}
	teamID, err := r.teams.EnsureByName(ctx, teamName)
	if err != nil {
		return false, err
	}
	return r.bindings.Ensure(ctx, u.ID, teamID, auth.RoleAdmin)
}

func (r *UserRepository) bySubject(ctx context.Context, subject string) (auth.User, error) {
	var u auth.User
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT id, email, display_name, disabled_at IS NOT NULL FROM users WHERE sso_subject = $1`, subject).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.Disabled)
	return u, r.db.Translate(err)
}
