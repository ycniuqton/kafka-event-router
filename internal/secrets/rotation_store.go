package secrets

import (
	"context"
)

// Rewrap swaps the wrapped data key of one envelope from an old master key to the current one.
type Rewrap struct {
	Ref         Ref
	FromKeyID   string
	FromWrapped []byte
	ToKeyID     string
	ToWrapped   []byte
}

// KeyUsage counts the envelopes wrapped by one master key.
type KeyUsage struct {
	Provider ProviderKind
	KeyID    string
	Count    int
}

// RotationRepository finds envelopes wrapped by a non-current key and re-wraps them atomically.
type RotationRepository interface {
	ListStale(ctx context.Context, provider ProviderKind, currentKeyID string, after Ref, limit int) ([]Envelope, error)
	Rewrap(ctx context.Context, rw Rewrap) (bool, error)
	KeyUsage(ctx context.Context) ([]KeyUsage, error)
}

// ListStale returns envelopes of provider whose key id differs from currentKeyID, in ref order after `after`.
func (r *PostgresRepository) ListStale(ctx context.Context, provider ProviderKind, currentKeyID string, after Ref, limit int) ([]Envelope, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT ref, provider, key_id, wrapped_key, nonce, ciphertext, created_at, rotated_at
		FROM secrets WHERE provider = $1 AND key_id <> $2 AND ref > $3 ORDER BY ref LIMIT $4`,
		string(provider), currentKeyID, string(after), limit)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	defer rows.Close()
	var out []Envelope
	for rows.Next() {
		var e Envelope
		var ref, kind string
		if err := rows.Scan(&ref, &kind, &e.KeyID, &e.WrappedKey, &e.Nonce, &e.Ciphertext, &e.CreatedAt, &e.RotatedAt); err != nil {
			return nil, r.db.Translate(err)
		}
		e.Ref, e.Provider = Ref(ref), ProviderKind(kind)
		out = append(out, e)
	}
	return out, r.db.Translate(rows.Err())
}

// Rewrap replaces the wrapped key only while the envelope still holds FromWrapped (a concurrent Replace wins).
func (r *PostgresRepository) Rewrap(ctx context.Context, rw Rewrap) (bool, error) {
	tag, err := r.db.Q(ctx).Exec(ctx, `UPDATE secrets SET key_id = $2, wrapped_key = $3, rotated_at = now()
		WHERE ref = $1 AND key_id = $4 AND wrapped_key = $5`,
		string(rw.Ref), rw.ToKeyID, rw.ToWrapped, rw.FromKeyID, rw.FromWrapped)
	if err != nil {
		return false, r.db.Translate(err)
	}
	return tag.RowsAffected() == 1, nil
}

// KeyUsage counts envelopes per provider and key id.
func (r *PostgresRepository) KeyUsage(ctx context.Context) ([]KeyUsage, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT provider, key_id, count(*) FROM secrets GROUP BY provider, key_id ORDER BY provider, key_id`)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	defer rows.Close()
	var out []KeyUsage
	for rows.Next() {
		var u KeyUsage
		var kind string
		if err := rows.Scan(&kind, &u.KeyID, &u.Count); err != nil {
			return nil, r.db.Translate(err)
		}
		u.Provider = ProviderKind(kind)
		out = append(out, u)
	}
	return out, r.db.Translate(rows.Err())
}
