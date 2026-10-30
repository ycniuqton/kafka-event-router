// Package statickey is the master-key provider backed by an operator-supplied base64 AES-256 key
// (ROUTER_SECRETS_MASTER_KEY or ROUTER_SECRETS_MASTER_KEY_FILE); allowed in every environment.
package statickey

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/secrets/keyring"
)

// StaticKeyIDPrefix marks key ids produced by this provider ("static:" + 16 hex chars of SHA-256).
const StaticKeyIDPrefix keyring.KeyIDPrefix = "static:"

// Provider wraps data keys with AES-256-GCM under the static master key; previous keys unwrap only,
// so `api secrets rotate` can re-wrap envelopes to the current key.
type Provider struct {
	*keyring.Ring
}

// Register adds the static_key provider to a registry.
func Register(reg *secrets.Registry) {
	reg.Register(secrets.ProviderStaticKey, func(_ context.Context, cfg config.SecretsConfig) (secrets.Provider, error) {
		return Open(cfg.Static)
	})
}

// Open decodes the current master key and the decrypt-only previous keys; any invalid key refuses start.
func Open(cfg config.StaticKeyConfig) (*Provider, error) {
	src := KeySource{Config: cfg}
	current, err := src.Current()
	if err != nil {
		return nil, err
	}
	previous, err := src.Previous()
	if err != nil {
		return nil, err
	}
	return &Provider{Ring: keyring.New(StaticKeyIDPrefix, current, previous...)}, nil
}

// Fingerprint returns the key id of a master key ("static:" + first hex chars of its SHA-256).
func Fingerprint(key []byte) string {
	return StaticKeyIDPrefix.Fingerprint(key)
}

// Kind implements secrets.Provider.
func (p *Provider) Kind() secrets.ProviderKind { return secrets.ProviderStaticKey }

// KeySource resolves master keys from inline base64 values and files holding base64 values.
type KeySource struct {
	Config config.StaticKeyConfig
}

// Current returns the current master key; exactly one of the inline key and the key file must be set.
func (s KeySource) Current() ([]byte, error) {
	c := s.Config
	if c.Key != "" && c.KeyFile != "" {
		return nil, fmt.Errorf("statickey: set only one of %s and %s", config.EnvSecretsMasterKey, config.EnvSecretsMasterKeyFile)
	}
	if c.KeyFile != "" {
		return KeyFile{Name: config.EnvSecretsMasterKeyFile, Path: c.KeyFile}.Load()
	}
	if c.Key == "" {
		return nil, fmt.Errorf("statickey: %s or %s is required", config.EnvSecretsMasterKey, config.EnvSecretsMasterKeyFile)
	}
	return EncodedKey{Name: config.EnvSecretsMasterKey, Value: c.Key}.Decode()
}

// Previous returns the decrypt-only keys: inline values first, then files.
func (s KeySource) Previous() ([][]byte, error) {
	var out [][]byte
	for _, v := range s.Config.PreviousKeys {
		key, err := EncodedKey{Name: config.EnvSecretsPreviousMasterKeys, Value: v}.Decode()
		if err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	for _, path := range s.Config.PreviousKeyFiles {
		key, err := KeyFile{Name: config.EnvSecretsPreviousMasterKeyFiles, Path: path}.Load()
		if err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, nil
}

// KeyFile is a file holding one base64 master key (surrounding whitespace ignored).
type KeyFile struct {
	Name config.EnvKey
	Path string
}

// Load reads and decodes the key file.
func (f KeyFile) Load() ([]byte, error) {
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, fmt.Errorf("statickey: %s: read %s: %w", f.Name, f.Path, err)
	}
	return EncodedKey{Name: f.Name, Value: string(bytes.TrimSpace(raw))}.Decode()
}

// EncodedKey is a base64 (standard, padded or unpadded) AES-256 master key named by its setting.
type EncodedKey struct {
	Name  config.EnvKey
	Value string
}

// Decode returns the 32 key bytes; the error never echoes the key material.
func (k EncodedKey) Decode() ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(k.Value)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(k.Value)
	}
	if err != nil {
		return nil, fmt.Errorf("statickey: %s is not valid base64", k.Name)
	}
	if len(key) != int(secrets.DataKeySize) {
		return nil, fmt.Errorf("statickey: %s must decode to %d bytes, got %d", k.Name, secrets.DataKeySize, len(key))
	}
	return key, nil
}
