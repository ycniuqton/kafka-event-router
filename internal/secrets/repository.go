package secrets

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/store"
)

// Envelope is one stored secret: ciphertext plus the data key wrapped by the master key.
type Envelope struct {
	Ref        Ref
	Provider   ProviderKind
	KeyID      string
	WrappedKey []byte
	Nonce      []byte
	Ciphertext []byte
	CreatedAt  time.Time
	RotatedAt  *time.Time
}

// SecretRepository persists envelopes.
type SecretRepository interface {
	Insert(ctx context.Context, e Envelope) error
	Get(ctx context.Context, ref Ref) (Envelope, error)
	Update(ctx context.Context, e Envelope) error
	Delete(ctx context.Context, ref Ref) error
}

// PostgresRepository stores envelopes in the secrets table (0003), joining the ambient tx.
type PostgresRepository struct {
	db *store.DB
}

// NewPostgresRepository builds the repository.
func NewPostgresRepository(db *store.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// Insert adds a new envelope.
func (r *PostgresRepository) Insert(ctx context.Context, e Envelope) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO secrets (ref, provider, key_id, wrapped_key, nonce, ciphertext)
		VALUES ($1, $2, $3, $4, $5, $6)`, string(e.Ref), string(e.Provider), e.KeyID, e.WrappedKey, e.Nonce, e.Ciphertext)
	return r.db.Translate(err)
}

// Get loads an envelope; unknown refs yield store.ErrNotFound.
func (r *PostgresRepository) Get(ctx context.Context, ref Ref) (Envelope, error) {
	e := Envelope{Ref: ref}
	var provider string
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT provider, key_id, wrapped_key, nonce, ciphertext, created_at, rotated_at
		FROM secrets WHERE ref = $1`, string(ref)).
		Scan(&provider, &e.KeyID, &e.WrappedKey, &e.Nonce, &e.Ciphertext, &e.CreatedAt, &e.RotatedAt)
	e.Provider = ProviderKind(provider)
	return e, r.db.Translate(err)
}

// Update replaces key material and ciphertext of an existing envelope.
func (r *PostgresRepository) Update(ctx context.Context, e Envelope) error {
	tag, err := r.db.Q(ctx).Exec(ctx, `UPDATE secrets SET provider = $2, key_id = $3, wrapped_key = $4, nonce = $5,
		ciphertext = $6, rotated_at = now() WHERE ref = $1`,
		string(e.Ref), string(e.Provider), e.KeyID, e.WrappedKey, e.Nonce, e.Ciphertext)
	if err != nil {
		return r.db.Translate(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// Delete removes an envelope (connections.secret_ref is set NULL by the FK).
func (r *PostgresRepository) Delete(ctx context.Context, ref Ref) error {
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM secrets WHERE ref = $1`, string(ref))
	return r.db.Translate(err)
}
