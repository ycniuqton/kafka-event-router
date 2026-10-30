package lookups

import (
	"context"

	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/pkg/lookup"
)

// SecretVault stores one envelope-encrypted SecretBundle per source through the secrets Manager
// (Vault Transit in production); calls join the caller's transaction.
type SecretVault struct {
	manager secrets.Manager
	codec   lookup.SecretCodec
}

// NewSecretVault wraps the secrets Manager.
func NewSecretVault(m secrets.Manager) *SecretVault { return &SecretVault{manager: m} }

// Store encrypts a bundle; an empty bundle stores nothing and returns an empty ref.
func (v *SecretVault) Store(ctx context.Context, b lookup.SecretBundle) (secrets.Ref, error) {
	if b.IsEmpty() {
		return "", nil
	}
	raw, err := v.codec.Encode(b)
	if err != nil {
		return "", err
	}
	defer clear(raw)
	return v.manager.Put(ctx, raw)
}

// Rewrite stores b as the full content of ref (an empty bundle drops it; no ref stores a new one).
func (v *SecretVault) Rewrite(ctx context.Context, ref secrets.Ref, b lookup.SecretBundle) (secrets.Ref, error) {
	if ref == "" {
		return v.Store(ctx, b)
	}
	if b.IsEmpty() {
		return "", v.manager.Delete(ctx, ref)
	}
	raw, err := v.codec.Encode(b)
	if err != nil {
		return "", err
	}
	defer clear(raw)
	return ref, v.manager.Replace(ctx, ref, raw)
}

// Load decrypts the bundle of ref; an empty ref yields an empty bundle.
func (v *SecretVault) Load(ctx context.Context, ref secrets.Ref) (lookup.SecretBundle, error) {
	if ref == "" {
		return lookup.SecretBundle{}, nil
	}
	raw, err := v.manager.Get(ctx, ref)
	if err != nil {
		return lookup.SecretBundle{}, err
	}
	defer clear(raw)
	return v.codec.Decode(raw)
}

// Drop deletes the bundle of ref (no-op for an empty ref).
func (v *SecretVault) Drop(ctx context.Context, ref secrets.Ref) error {
	if ref == "" {
		return nil
	}
	return v.manager.Delete(ctx, ref)
}
