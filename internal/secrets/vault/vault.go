// Package vault is the production master-key provider using Vault's Transit secrets engine.
package vault

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/internal/secrets"
)

// APIConst names Vault HTTP API constants.
type APIConst string

const (
	PathPrefix  APIConst = "/v1/"
	OpEncrypt   APIConst = "/encrypt/"
	OpDecrypt   APIConst = "/decrypt/"
	HeaderToken APIConst = "X-Vault-Token"
	KeyIDPrefix APIConst = "vault:"
)

// RequestTimeout bounds one Transit call.
type RequestTimeout time.Duration

const DefaultRequestTimeout RequestTimeout = RequestTimeout(10 * time.Second)

// Provider wraps data keys with Vault Transit encrypt/decrypt.
type Provider struct {
	cfg    config.VaultConfig
	client *http.Client
}

// Register adds the vault provider to a registry.
func Register(reg *secrets.Registry) {
	reg.Register(secrets.ProviderVault, func(_ context.Context, cfg config.SecretsConfig) (secrets.Provider, error) {
		return New(cfg.Vault, nil)
	})
}

// New builds the provider; client may be nil for a default client with timeout.
func New(cfg config.VaultConfig, client *http.Client) (*Provider, error) {
	if cfg.Addr == "" || cfg.Token == "" || cfg.TransitKey == "" {
		return nil, errors.New("vault: ROUTER_VAULT_ADDR, ROUTER_VAULT_TOKEN and ROUTER_VAULT_TRANSIT_KEY are required")
	}
	if client == nil {
		client = &http.Client{Timeout: time.Duration(DefaultRequestTimeout)}
	}
	return &Provider{cfg: cfg, client: client}, nil
}

// Kind implements secrets.Provider.
func (p *Provider) Kind() secrets.ProviderKind { return secrets.ProviderVault }

// KeyID implements secrets.Provider (key versions travel inside the ciphertext).
func (p *Provider) KeyID() string { return string(KeyIDPrefix) + p.cfg.TransitKey }

// WrapKey encrypts dek with the Transit key; the stored value is Vault's "vault:vN:…" ciphertext.
func (p *Provider) WrapKey(ctx context.Context, dek []byte) ([]byte, error) {
	var out transitResponse
	in := transitRequest{Plaintext: base64.StdEncoding.EncodeToString(dek)}
	if err := p.call(ctx, OpEncrypt, p.cfg.TransitKey, in, &out); err != nil {
		return nil, err
	}
	return []byte(out.Data.Ciphertext), nil
}

// UnwrapKey decrypts with the Transit key named in keyID.
func (p *Provider) UnwrapKey(ctx context.Context, wrapped []byte, keyID string) ([]byte, error) {
	keyName := strings.TrimPrefix(keyID, string(KeyIDPrefix))
	var out transitResponse
	if err := p.call(ctx, OpDecrypt, keyName, transitRequest{Ciphertext: string(wrapped)}, &out); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Data.Plaintext)
}

func (p *Provider) call(ctx context.Context, op APIConst, key string, in transitRequest, out *transitResponse) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	url := strings.TrimRight(p.cfg.Addr, "/") + string(PathPrefix) + p.cfg.Mount + string(op) + key
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set(string(HeaderToken), p.cfg.Token)
	req.Header.Set(string(httpserver.HeaderContentType), string(httpserver.MediaTypeJSON))
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault: %s: %w", op, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vault: %s: status %d", op, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type transitRequest struct {
	Plaintext  string `json:"plaintext,omitempty"`
	Ciphertext string `json:"ciphertext,omitempty"`
}

type transitResponse struct {
	Data struct {
		Ciphertext string `json:"ciphertext"`
		Plaintext  string `json:"plaintext"`
	} `json:"data"`
}
