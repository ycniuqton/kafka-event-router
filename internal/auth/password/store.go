package password

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// Credential is a user's stored password together with the user fields login needs.
type Credential struct {
	UserID      uuid.UUID
	Email       string
	DisplayName string
	Subject     string
	Disabled    bool
	Hash        string
	MustChange  bool
}

// CredentialStore persists password credentials (table user_passwords).
type CredentialStore interface {
	FindByEmail(ctx context.Context, email string) (Credential, error)
	Get(ctx context.Context, userID uuid.UUID) (Credential, error)
	Set(ctx context.Context, userID uuid.UUID, hash string, mustChange bool) error
	Any(ctx context.Context) (bool, error)
	EnsureSubject(ctx context.Context, userID uuid.UUID) error
}

// SubjectPrefix marks the sso_subject of users created for password login.
type SubjectPrefix string

const PasswordSubjectPrefix SubjectPrefix = "password:"

// PostgresCredentialStore implements CredentialStore.
type PostgresCredentialStore struct {
	db *store.DB
}

// NewPostgresCredentialStore builds the store.
func NewPostgresCredentialStore(db *store.DB) *PostgresCredentialStore {
	return &PostgresCredentialStore{db: db}
}

// credentialSQL is the projection shared by credential reads.
const credentialSQL = `SELECT u.id, u.email, u.display_name, COALESCE(u.sso_subject, ''), u.disabled_at IS NOT NULL,
	p.hash, p.must_change FROM users u JOIN user_passwords p ON p.user_id = u.id`

// FindByEmail returns the credential of the user with email (case-insensitive), or store.ErrNotFound.
func (s *PostgresCredentialStore) FindByEmail(ctx context.Context, email string) (Credential, error) {
	return s.one(ctx, credentialSQL+` WHERE lower(u.email) = lower($1)`, email)
}

// Get returns the credential of a user, or store.ErrNotFound when the user has no password.
func (s *PostgresCredentialStore) Get(ctx context.Context, userID uuid.UUID) (Credential, error) {
	return s.one(ctx, credentialSQL+` WHERE u.id = $1`, userID)
}

// Set creates or replaces the user's password hash and its forced-change flag.
func (s *PostgresCredentialStore) Set(ctx context.Context, userID uuid.UUID, hash string, mustChange bool) error {
	_, err := s.db.Q(ctx).Exec(ctx, `INSERT INTO user_passwords (user_id, hash, must_change) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET hash = EXCLUDED.hash, must_change = EXCLUDED.must_change, changed_at = now()`,
		userID, hash, mustChange)
	return s.db.Translate(err)
}

// Any reports whether at least one password credential exists.
func (s *PostgresCredentialStore) Any(ctx context.Context) (bool, error) {
	var ok bool
	err := s.db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_passwords)`).Scan(&ok)
	return ok, s.db.Translate(err)
}

// EnsureSubject gives a pending (invited) user the password login subject so the login can provision it.
func (s *PostgresCredentialStore) EnsureSubject(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Q(ctx).Exec(ctx, `UPDATE users SET sso_subject = $2 || id::text WHERE id = $1 AND sso_subject IS NULL`,
		userID, string(PasswordSubjectPrefix))
	return s.db.Translate(err)
}

func (s *PostgresCredentialStore) one(ctx context.Context, sql string, arg any) (Credential, error) {
	var c Credential
	err := s.db.Q(ctx).QueryRow(ctx, sql, arg).Scan(&c.UserID, &c.Email, &c.DisplayName, &c.Subject, &c.Disabled, &c.Hash, &c.MustChange)
	return c, s.db.Translate(err)
}
