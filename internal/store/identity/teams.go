package identity

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// TeamRepository manages the teams rows auth needs.
type TeamRepository struct {
	db *store.DB
}

// EnsureByName returns the id of the team with name, creating it if missing.
func (r *TeamRepository) EnsureByName(ctx context.Context, name string) (uuid.UUID, error) {
	q := r.db.Q(ctx)
	if _, err := q.Exec(ctx, `INSERT INTO teams (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, name); err != nil {
		return uuid.Nil, r.db.Translate(err)
	}
	var id uuid.UUID
	if err := q.QueryRow(ctx, `SELECT id FROM teams WHERE name = $1`, name).Scan(&id); err != nil {
		return uuid.Nil, r.db.Translate(err)
	}
	return id, nil
}

// FindByName returns the id of an existing team (found=false when no team has that name).
func (r *TeamRepository) FindByName(ctx context.Context, name string) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT id FROM teams WHERE name = $1`, name).Scan(&id)
	if errors.Is(r.db.Translate(err), store.ErrNotFound) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, r.db.Translate(err)
	}
	return id, true, nil
}
