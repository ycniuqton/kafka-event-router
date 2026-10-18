// Package local is the dev-only master-key provider backed by a key file (generated on first run).
package local

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/secrets"
)

// FileMode names permissions of the key file and its directory.
type FileMode fs.FileMode

const (
	KeyFileMode FileMode = 0o600
	KeyDirMode  FileMode = 0o700
)

// KeyIDPrefix marks key ids produced by this provider.
type KeyIDPrefix string

const LocalKeyIDPrefix KeyIDPrefix = "local:"

// KeyIDHexLen is how many hex chars of the key fingerprint form the key id.
type KeyIDHexLen int

const FingerprintLen KeyIDHexLen = 16

// NonceLen is the AES-GCM nonce length prefixed to wrapped keys.
type NonceLen int

const GCMNonceSize NonceLen = 12

// Provider wraps data keys with AES-256-GCM under a master key read from a file; previous master keys
// stay available for unwrapping only, so `api secrets rotate` can re-wrap envelopes to the current key.
type Provider struct {
	key      []byte
	keyID    string
	previous map[string][]byte
	cipher   secrets.AESGCMCipher
}

// Register adds the local provider to a registry.
func Register(reg *secrets.Registry) {
	reg.Register(secrets.ProviderLocal, func(_ context.Context, cfg config.SecretsConfig) (secrets.Provider, error) {
		return Open(cfg.LocalKeyFile, cfg.Rotation.LocalPreviousKeyFiles...)
	})
}

// Open loads the current hex key file (created when missing) and the decrypt-only previous key files.
func Open(path string, previous ...string) (*Provider, error) {
	key, err := KeyFile{Path: path}.LoadOrCreate()
	if err != nil {
		return nil, err
	}
	p := &Provider{key: key, keyID: Fingerprint(key), previous: map[string][]byte{}}
	for _, old := range previous {
		oldKey, err := KeyFile{Path: old}.Load()
		if err != nil {
			return nil, err
		}
		p.previous[Fingerprint(oldKey)] = oldKey
	}
	return p, nil
}

// Fingerprint returns the key id of a master key ("local:" + first hex chars of its SHA-256).
func Fingerprint(key []byte) string {
	sum := sha256.Sum256(key)
	return string(LocalKeyIDPrefix) + hex.EncodeToString(sum[:])[:FingerprintLen]
}

// Kind implements secrets.Provider.
func (p *Provider) Kind() secrets.ProviderKind { return secrets.ProviderLocal }

// KeyID implements secrets.Provider.
func (p *Provider) KeyID() string { return p.keyID }

// WrapKey seals dek as nonce||ciphertext.
func (p *Provider) WrapKey(_ context.Context, dek []byte) ([]byte, error) {
	nonce, ct, err := p.cipher.Seal(p.key, dek)
	if err != nil {
		return nil, err
	}
	return append(nonce, ct...), nil
}

// UnwrapKey opens a wrapped key with the current or a previous master key named by keyID.
func (p *Provider) UnwrapKey(_ context.Context, wrapped []byte, keyID string) ([]byte, error) {
	key, ok := p.keyFor(keyID)
	if !ok {
		return nil, fmt.Errorf("local: key %q not available (have %q)", keyID, p.keyID)
	}
	if len(wrapped) < int(GCMNonceSize) {
		return nil, secrets.ErrDecrypt
	}
	return p.cipher.Open(key, wrapped[:GCMNonceSize], wrapped[GCMNonceSize:])
}

func (p *Provider) keyFor(keyID string) ([]byte, bool) {
	if keyID == p.keyID {
		return p.key, true
	}
	key, ok := p.previous[keyID]
	return key, ok
}

// KeyFile reads or generates the hex-encoded master key.
type KeyFile struct {
	Path string
}

// LoadOrCreate returns the key, generating a 32-byte key file (0600) on first use.
func (f KeyFile) LoadOrCreate() ([]byte, error) {
	_, err := os.Stat(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return f.create()
	}
	return f.Load()
}

// Load reads an existing hex-encoded key file.
func (f KeyFile) Load() ([]byte, error) {
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, fmt.Errorf("local: read key file: %w", err)
	}
	key, err := hex.DecodeString(string(bytes.TrimSpace(raw)))
	if err != nil || len(key) != int(secrets.DataKeySize) {
		return nil, fmt.Errorf("local: key file %s must hold %d hex-encoded bytes", f.Path, secrets.DataKeySize)
	}
	return key, nil
}

func (f KeyFile) create() ([]byte, error) {
	key := make([]byte, secrets.DataKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), fs.FileMode(KeyDirMode)); err != nil {
		return nil, fmt.Errorf("local: create key dir: %w", err)
	}
	if err := os.WriteFile(f.Path, []byte(hex.EncodeToString(key)), fs.FileMode(KeyFileMode)); err != nil {
		return nil, fmt.Errorf("local: write key file: %w", err)
	}
	return key, nil
}
