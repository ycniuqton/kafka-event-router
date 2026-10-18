package connections

import (
	"context"
	"errors"

	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/pkg/kafka"
)

// SecretVault stores one envelope-encrypted SecretBundle per connection through the secrets Manager.
// Calls join the caller's transaction, so a rolled-back save never leaves an orphan secret.
type SecretVault struct {
	manager secrets.Manager
	codec   kafka.SecretCodec
}

// NewSecretVault wraps the secrets Manager.
func NewSecretVault(manager secrets.Manager) *SecretVault {
	return &SecretVault{manager: manager}
}

// Store encrypts a bundle; an empty bundle stores nothing and returns an empty ref.
func (v *SecretVault) Store(ctx context.Context, bundle kafka.SecretBundle) (secrets.Ref, error) {
	if bundle.IsEmpty() {
		return "", nil
	}
	raw, err := v.codec.Encode(bundle)
	if err != nil {
		return "", err
	}
	defer clear(raw)
	return v.manager.Put(ctx, raw)
}

// Replace overlays patch onto the stored bundle and re-encrypts it (keeping the ref when one exists).
func (v *SecretVault) Replace(ctx context.Context, ref secrets.Ref, patch kafka.SecretBundle) (secrets.Ref, kafka.SecretBundle, error) {
	current, err := v.Load(ctx, ref)
	if err != nil {
		return "", kafka.SecretBundle{}, err
	}
	merged := current.Merge(patch)
	if ref == "" {
		newRef, err := v.Store(ctx, merged)
		return newRef, merged, err
	}
	raw, err := v.codec.Encode(merged)
	if err != nil {
		return "", kafka.SecretBundle{}, err
	}
	defer clear(raw)
	return ref, merged, v.manager.Replace(ctx, ref, raw)
}

// Rewrite stores bundle as the full content of ref (unused fields removed); without a ref it stores a new one.
func (v *SecretVault) Rewrite(ctx context.Context, ref secrets.Ref, bundle kafka.SecretBundle) (secrets.Ref, error) {
	if ref == "" {
		return v.Store(ctx, bundle)
	}
	raw, err := v.codec.Encode(bundle)
	if err != nil {
		return "", err
	}
	defer clear(raw)
	return ref, v.manager.Replace(ctx, ref, raw)
}

// Load decrypts the bundle of ref; an empty ref yields an empty bundle.
func (v *SecretVault) Load(ctx context.Context, ref secrets.Ref) (kafka.SecretBundle, error) {
	if ref == "" {
		return kafka.SecretBundle{}, nil
	}
	raw, err := v.manager.Get(ctx, ref)
	if errors.Is(err, secrets.ErrUnavailable) {
		return kafka.SecretBundle{}, err
	}
	if err != nil {
		return kafka.SecretBundle{}, ErrSecretUnavailable.Wrap(err)
	}
	defer clear(raw)
	return v.codec.Decode(raw)
}

// Drop deletes the stored bundle of ref (no-op for an empty ref).
func (v *SecretVault) Drop(ctx context.Context, ref secrets.Ref) error {
	if ref == "" {
		return nil
	}
	return v.manager.Delete(ctx, ref)
}
