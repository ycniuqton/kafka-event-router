// Package keyring is the AES-256-GCM master-key ring shared by the in-process master-key providers
// (local key file, static key): one current key wraps, previous keys unwrap only.
package keyring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/eventrouter/router/internal/secrets"
)

// KeyIDPrefix marks key ids produced by one provider kind (e.g. "local:", "static:").
type KeyIDPrefix string

// KeyIDHexLen is how many hex chars of the key fingerprint form the key id.
type KeyIDHexLen int

const FingerprintLen KeyIDHexLen = 16

// NonceLen is the AES-GCM nonce length prefixed to wrapped keys.
type NonceLen int

const GCMNonceSize NonceLen = 12

// Ring wraps data keys under the current master key and unwraps with the current or a previous key by id.
type Ring struct {
	prefix   KeyIDPrefix
	key      []byte
	keyID    string
	previous map[string][]byte
	cipher   secrets.AESGCMCipher
}

// New builds a ring over the current key and decrypt-only previous keys.
func New(prefix KeyIDPrefix, current []byte, previous ...[]byte) *Ring {
	r := &Ring{prefix: prefix, key: current, keyID: prefix.Fingerprint(current), previous: map[string][]byte{}}
	for _, old := range previous {
		r.previous[prefix.Fingerprint(old)] = old
	}
	return r
}

// Fingerprint returns the key id of a master key (prefix + first hex chars of its SHA-256).
func (p KeyIDPrefix) Fingerprint(key []byte) string {
	sum := sha256.Sum256(key)
	return string(p) + hex.EncodeToString(sum[:])[:FingerprintLen]
}

// KeyID returns the id of the current master key.
func (r *Ring) KeyID() string { return r.keyID }

// WrapKey seals dek as nonce||ciphertext under the current key.
func (r *Ring) WrapKey(_ context.Context, dek []byte) ([]byte, error) {
	nonce, ct, err := r.cipher.Seal(r.key, dek)
	if err != nil {
		return nil, err
	}
	return append(nonce, ct...), nil
}

// UnwrapKey opens a wrapped key with the current or a previous master key named by keyID.
func (r *Ring) UnwrapKey(_ context.Context, wrapped []byte, keyID string) ([]byte, error) {
	key, ok := r.keyFor(keyID)
	if !ok {
		return nil, fmt.Errorf("%s key %q not available (have %q)", r.prefix, keyID, r.keyID)
	}
	if len(wrapped) < int(GCMNonceSize) {
		return nil, secrets.ErrDecrypt
	}
	return r.cipher.Open(key, wrapped[:GCMNonceSize], wrapped[GCMNonceSize:])
}

func (r *Ring) keyFor(keyID string) ([]byte, bool) {
	if keyID == r.keyID {
		return r.key, true
	}
	key, ok := r.previous[keyID]
	return key, ok
}
