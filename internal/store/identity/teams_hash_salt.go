package identity

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// HashSaltRef returns the secret ref of the team's hash salt (migration 0006); ok is false when the
// team has none yet. An unknown or malformed team id is store.ErrNotFound.
func (r *TeamRepository) HashSaltRef(ctx context.Context, teamID string) (string, bool, error) {
	id, err := uuid.Parse(teamID)
	if err != nil {
		return "", false, store.ErrNotFound
	}
	var ref *string
	if err := r.db.Q(ctx).QueryRow(ctx, `SELECT hash_salt_ref FROM teams WHERE id = $1`, id).Scan(&ref); err != nil {
		return "", false, r.db.Translate(err)
	}
	if ref == nil {
		return "", false, nil
	}
	return *ref, true, nil
}

// SetHashSaltRefIfEmpty stores ref unless another writer was first and returns the ref that won.
func (r *TeamRepository) SetHashSaltRefIfEmpty(ctx context.Context, teamID, ref string) (string, error) {
	id, err := uuid.Parse(teamID)
	if err != nil {
		return "", store.ErrNotFound
	}
	q := r.db.Q(ctx)
	if _, err := q.Exec(ctx, `UPDATE teams SET hash_salt_ref = $2 WHERE id = $1 AND hash_salt_ref IS NULL`, id, ref); err != nil {
		return "", r.db.Translate(err)
	}
	winner, ok, err := r.HashSaltRef(ctx, teamID)
	if err == nil && !ok {
		return "", store.ErrNotFound
	}
	return winner, err
}
