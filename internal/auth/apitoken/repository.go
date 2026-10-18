package apitoken

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/store"
)

// Repository persists API tokens.
type Repository interface {
	Insert(ctx context.Context, t Token) (Token, error)
	ListActive(ctx context.Context, userID uuid.UUID) ([]Token, error)
	CountActive(ctx context.Context, userID uuid.UUID) (int, error)
	FindByHash(ctx context.Context, hash []byte) (Token, error)
	Revoke(ctx context.Context, userID, id uuid.UUID, at time.Time) (Token, error)
	TouchLastUsed(ctx context.Context, used map[uuid.UUID]time.Time) error
}

// Constraint names the api_tokens constraints mapped to request fields.
type Constraint string

const ConstraintActiveName Constraint = "api_tokens_user_name_active_key"

// tokenColumns is the select list every query scans with scanToken.
const tokenColumns = `id, user_id, name, prefix, scopes, expires_at, last_used_at, revoked_at, created_at, token_hash`

// PostgresRepository implements Repository on the api_tokens table (migration 1000).
type PostgresRepository struct {
	db *store.DB
}

// NewPostgresRepository builds the repository and maps the unique-name constraint to /name.
func NewPostgresRepository(db *store.DB) *PostgresRepository {
	db.Constraints().Register(string(ConstraintActiveName), string(FieldName))
	return &PostgresRepository{db: db}
}

// Insert stores t and returns it with id and created_at.
func (r *PostgresRepository) Insert(ctx context.Context, t Token) (Token, error) {
	row := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO api_tokens (user_id, name, token_hash, prefix, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+tokenColumns, t.UserID, t.Name, t.Hash, t.Prefix, ScopeList(t.Scopes).Strings(), t.ExpiresAt)
	return r.scan(row)
}

// ListActive returns the user's non-revoked tokens, newest first.
func (r *PostgresRepository) ListActive(ctx context.Context, userID uuid.UUID) ([]Token, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+tokenColumns+` FROM api_tokens
		WHERE user_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	defer rows.Close()
	out := []Token{}
	for rows.Next() {
		t, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, r.db.Translate(rows.Err())
}

// CountActive counts the user's non-revoked tokens.
func (r *PostgresRepository) CountActive(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM api_tokens WHERE user_id = $1 AND revoked_at IS NULL`, userID).Scan(&n)
	return n, r.db.Translate(err)
}

// FindByHash returns the token with the given secret hash (revoked ones included).
func (r *PostgresRepository) FindByHash(ctx context.Context, hash []byte) (Token, error) {
	return r.scan(r.db.Q(ctx).QueryRow(ctx, `SELECT `+tokenColumns+` FROM api_tokens WHERE token_hash = $1`, hash))
}

// Revoke marks the user's token revoked; an unknown, foreign or already revoked id is store.ErrNotFound.
func (r *PostgresRepository) Revoke(ctx context.Context, userID, id uuid.UUID, at time.Time) (Token, error) {
	return r.scan(r.db.Q(ctx).QueryRow(ctx, `UPDATE api_tokens SET revoked_at = $3
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL RETURNING `+tokenColumns, id, userID, at))
}

// TouchLastUsed writes a batch of last-used times in one statement (never moves a time backwards).
func (r *PostgresRepository) TouchLastUsed(ctx context.Context, used map[uuid.UUID]time.Time) error {
	ids, times := make([]uuid.UUID, 0, len(used)), make([]time.Time, 0, len(used))
	for id, at := range used {
		ids, times = append(ids, id), append(times, at)
	}
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE api_tokens t SET last_used_at = u.at
		FROM unnest($1::uuid[], $2::timestamptz[]) AS u(id, at)
		WHERE t.id = u.id AND (t.last_used_at IS NULL OR t.last_used_at < u.at)`, ids, times)
	return r.db.Translate(err)
}

// rowScanner is satisfied by pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func (r *PostgresRepository) scan(row rowScanner) (Token, error) {
	var t Token
	var scopes []string
	err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.Prefix, &scopes, &t.ExpiresAt, &t.LastUsedAt, &t.RevokedAt, &t.CreatedAt, &t.Hash)
	if err != nil {
		return Token{}, r.db.Translate(err)
	}
	t.Scopes = ScopeList{}.Parse(scopes)
	return t, nil
}

// ScopeList converts between token scopes and their stored text[] form.
type ScopeList []auth.TokenScope

// Strings returns the scopes as text.
func (l ScopeList) Strings() []string {
	out := make([]string, 0, len(l))
	for _, s := range l {
		out = append(out, string(s))
	}
	return out
}

// Parse converts stored text values into scopes.
func (ScopeList) Parse(values []string) []auth.TokenScope {
	out := make([]auth.TokenScope, 0, len(values))
	for _, v := range values {
		out = append(out, auth.TokenScope(v))
	}
	return out
}
