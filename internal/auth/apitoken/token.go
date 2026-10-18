// Package apitoken issues, lists and revokes user-owned API tokens for automation and authenticates
// requests carrying `Authorization: Bearer ert_…`. Only SHA-256 hashes are stored.
package apitoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
)

// Format describes the shape of a token secret: `ert_` + base64url(32 random bytes).
type Format string

const SecretPrefix Format = "ert_"

// FormatSize groups the byte/character sizes of the token format.
type FormatSize int

const (
	SecretBytes        FormatSize = 32
	DisplayPrefixChars FormatSize = 8
)

// Status tells whether a listed token can still authenticate.
type Status string

const (
	StatusActive  Status = "active"
	StatusExpired Status = "expired"
)

// Token is one stored API token (never its secret).
type Token struct {
	ID         uuid.UUID         `json:"id"`
	UserID     uuid.UUID         `json:"-"`
	Name       string            `json:"name"`
	Prefix     string            `json:"prefix"`
	Scopes     []auth.TokenScope `json:"scopes"`
	ExpiresAt  *time.Time        `json:"expires_at"`
	LastUsedAt *time.Time        `json:"last_used_at"`
	RevokedAt  *time.Time        `json:"-"`
	CreatedAt  time.Time         `json:"created_at"`
	Hash       []byte            `json:"-"`
}

// StatusAt returns whether the token is active or expired at now.
func (t Token) StatusAt(now time.Time) Status {
	if t.ExpiresAt != nil && !now.Before(*t.ExpiresAt) {
		return StatusExpired
	}
	return StatusActive
}

// Usable reports whether the token may authenticate at now.
func (t Token) Usable(now time.Time) bool {
	return t.RevokedAt == nil && t.StatusAt(now) == StatusActive
}

// Secret is a freshly generated token value plus what is stored about it.
type Secret struct {
	Value  string
	Hash   []byte
	Prefix string
}

// SecretCodec generates token secrets and hashes presented ones.
type SecretCodec struct{}

// Generate returns a new random secret.
func (c SecretCodec) Generate() Secret {
	b := make([]byte, SecretBytes)
	_, _ = rand.Read(b)
	value := string(SecretPrefix) + base64.RawURLEncoding.EncodeToString(b)
	return Secret{Value: value, Hash: c.Hash(value), Prefix: value[:len(SecretPrefix)+int(DisplayPrefixChars)]}
}

// Hash returns the SHA-256 lookup key of a presented secret.
func (SecretCodec) Hash(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

// Recognises reports whether a bearer value has the API-token shape (other bearers are not ours).
func (SecretCodec) Recognises(value string) bool {
	return strings.HasPrefix(value, string(SecretPrefix)) && len(value) > len(SecretPrefix)
}
