package ctxhelpers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// Hasher returns a stable salted hash of a value.
type Hasher interface {
	Hash(ctx context.Context, v any) (string, error)
}

// SaltProvider returns the per-team hash salt (implemented by js-sandbox on platform-core secrets).
type SaltProvider interface {
	Salt(ctx context.Context, teamID string) ([]byte, error)
}

// StaticSaltProvider returns the same salt for every team (tests, routerctl).
type StaticSaltProvider struct {
	Value []byte
}

// Salt returns the configured value.
func (p StaticSaltProvider) Salt(context.Context, string) ([]byte, error) { return p.Value, nil }

// SaltedHasher computes hex SHA-256(salt || text(value)); the salt is fetched once per team.
type SaltedHasher struct {
	provider SaltProvider
	teamID   string
	mu       sync.Mutex
	salt     []byte
	loaded   bool
}

// NewSaltedHasher binds a provider to one team.
func NewSaltedHasher(p SaltProvider, teamID string) *SaltedHasher {
	return &SaltedHasher{provider: p, teamID: teamID}
}

// Hash returns the hex digest; nil stays nil-equivalent empty input.
func (h *SaltedHasher) Hash(ctx context.Context, v any) (string, error) {
	salt, err := h.saltFor(ctx)
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	sum.Write(salt)
	sum.Write([]byte(TextForm{}.Of(v)))
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func (h *SaltedHasher) saltFor(ctx context.Context) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.loaded {
		return h.salt, nil
	}
	salt, err := h.provider.Salt(ctx, h.teamID)
	if err != nil {
		return nil, err
	}
	h.salt, h.loaded = salt, true
	return salt, nil
}
