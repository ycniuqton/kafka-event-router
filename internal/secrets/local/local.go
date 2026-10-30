// Package local is the dev-only master-key provider backed by a key file (generated on first run).
package local

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/secrets/keyring"
)

// FileMode names permissions of the key file and its directory.
type FileMode fs.FileMode

const (
	KeyFileMode FileMode = 0o600
	KeyDirMode  FileMode = 0o700
)

// LocalKeyIDPrefix marks key ids produced by this provider.
const LocalKeyIDPrefix keyring.KeyIDPrefix = "local:"

// Provider wraps data keys with AES-256-GCM under a master key read from a file; previous master keys
// stay available for unwrapping only, so `api secrets rotate` can re-wrap envelopes to the current key.
type Provider struct {
	*keyring.Ring
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
	var old [][]byte
	for _, prev := range previous {
		oldKey, err := KeyFile{Path: prev}.Load()
		if err != nil {
			return nil, err
		}
		old = append(old, oldKey)
	}
	return &Provider{Ring: keyring.New(LocalKeyIDPrefix, key, old...)}, nil
}

// Fingerprint returns the key id of a master key ("local:" + first hex chars of its SHA-256).
func Fingerprint(key []byte) string {
	return LocalKeyIDPrefix.Fingerprint(key)
}

// Kind implements secrets.Provider.
func (p *Provider) Kind() secrets.ProviderKind { return secrets.ProviderLocal }

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
