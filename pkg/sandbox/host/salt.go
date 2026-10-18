package host

import (
	"context"
	"crypto/rand"
	"sync"
)

// SaltSize is the byte length of a generated team hash salt.
type SaltSize int

const SaltSizeDefault SaltSize = 32

// SaltProvider returns the per-team hash salt (matches the engine's ctxhelpers.SaltProvider port).
type SaltProvider interface {
	Salt(ctx context.Context, teamID string) ([]byte, error)
}

// SecretStore stores salts as secrets (adapter over internal/secrets.Manager).
type SecretStore interface {
	Put(ctx context.Context, plaintext []byte) (string, error)
	Get(ctx context.Context, ref string) ([]byte, error)
}

// TeamSaltRefStore reads and sets teams.hash_salt_ref (migration 0006).
type TeamSaltRefStore interface {
	HashSaltRef(ctx context.Context, teamID string) (ref string, ok bool, err error)
	SetHashSaltRefIfEmpty(ctx context.Context, teamID, ref string) (winner string, err error)
}

// StoreSaltProvider resolves salts via secrets, generating one on first use; results are cached.
type StoreSaltProvider struct {
	secrets SecretStore
	refs    TeamSaltRefStore
	size    SaltSize
	mu      sync.Mutex
	cache   map[string][]byte
}

// NewStoreSaltProvider wires the secret store and the team ref store.
func NewStoreSaltProvider(secrets SecretStore, refs TeamSaltRefStore) *StoreSaltProvider {
	return &StoreSaltProvider{secrets: secrets, refs: refs, size: SaltSizeDefault, cache: map[string][]byte{}}
}

// Salt returns the team salt, creating and storing it when the team has none yet.
func (p *StoreSaltProvider) Salt(ctx context.Context, teamID string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.cache[teamID]; ok {
		return s, nil
	}
	ref, err := p.ensureRef(ctx, teamID)
	if err != nil {
		return nil, err
	}
	salt, err := p.secrets.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	p.cache[teamID] = salt
	return salt, nil
}

// ensureRef returns the stored ref or generates a salt; a concurrent winner's ref is kept.
func (p *StoreSaltProvider) ensureRef(ctx context.Context, teamID string) (string, error) {
	ref, ok, err := p.refs.HashSaltRef(ctx, teamID)
	if err != nil || ok {
		return ref, err
	}
	salt := make([]byte, p.size)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	created, err := p.secrets.Put(ctx, salt)
	if err != nil {
		return "", err
	}
	return p.refs.SetHashSaltRefIfEmpty(ctx, teamID, created)
}
