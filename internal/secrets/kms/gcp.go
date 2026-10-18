package kms

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/internal/secrets"
)

// GCPAPI names Cloud KMS REST API constants.
type GCPAPI string

const (
	GCPDefaultEndpoint GCPAPI = "https://cloudkms.googleapis.com"
	GCPVersionPath     GCPAPI = "/v1/"
	GCPOpEncrypt       GCPAPI = ":encrypt"
	GCPOpDecrypt       GCPAPI = ":decrypt"
	GCPScope           GCPAPI = "https://www.googleapis.com/auth/cloudkms"
)

// GCPRequestTimeout bounds one Cloud KMS call.
type GCPRequestTimeout time.Duration

const DefaultGCPRequestTimeout GCPRequestTimeout = GCPRequestTimeout(10 * time.Second)

// GCPProvider wraps data keys with a Cloud KMS symmetric key over the REST API.
type GCPProvider struct {
	client   *http.Client
	endpoint string
	keyName  string
}

// NewGCPFromConfig builds the provider with Application Default Credentials or a credentials file.
func NewGCPFromConfig(ctx context.Context, cfg config.KMSConfig) (*GCPProvider, error) {
	if cfg.GCPKeyName == "" {
		return nil, errors.New("kms: ROUTER_GCP_KMS_KEY_NAME is required for gcp_kms")
	}
	creds, err := GCPCredentials{File: cfg.GCPCredentialsFile}.Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("kms: gcp credentials: %w", err)
	}
	client := &http.Client{Timeout: time.Duration(DefaultGCPRequestTimeout), Transport: creds}
	return NewGCP(client, cfg.GCPEndpoint, cfg.GCPKeyName), nil
}

// NewGCP builds the provider over an authorised HTTP client; endpoint "" means the public API.
func NewGCP(client *http.Client, endpoint, keyName string) *GCPProvider {
	if endpoint == "" {
		endpoint = string(GCPDefaultEndpoint)
	}
	return &GCPProvider{client: client, endpoint: strings.TrimRight(endpoint, "/"), keyName: keyName}
}

// Kind implements secrets.Provider.
func (p *GCPProvider) Kind() secrets.ProviderKind { return config.SecretsGCPKMS }

// KeyID implements secrets.Provider (the key version is embedded in the ciphertext).
func (p *GCPProvider) KeyID() string { return string(PrefixGCP) + p.keyName }

// WrapKey encrypts dek under the configured key.
func (p *GCPProvider) WrapKey(ctx context.Context, dek []byte) ([]byte, error) {
	var out gcpResponse
	if err := p.call(ctx, p.keyName, GCPOpEncrypt, gcpRequest{Plaintext: base64.StdEncoding.EncodeToString(dek)}, &out); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Ciphertext)
}

// UnwrapKey decrypts with the key recorded in keyID.
func (p *GCPProvider) UnwrapKey(ctx context.Context, wrapped []byte, keyID string) ([]byte, error) {
	var out gcpResponse
	in := gcpRequest{Ciphertext: base64.StdEncoding.EncodeToString(wrapped)}
	if err := p.call(ctx, PrefixGCP.Trim(keyID), GCPOpDecrypt, in, &out); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Plaintext)
}

func (p *GCPProvider) call(ctx context.Context, keyName string, op GCPAPI, in gcpRequest, out *gcpResponse) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	url := p.endpoint + string(GCPVersionPath) + keyName + string(op)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set(string(httpserver.HeaderContentType), string(httpserver.MediaTypeJSON))
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("kms: gcp %s: %w", op, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("kms: gcp %s: status %d", op, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type gcpRequest struct {
	Plaintext  string `json:"plaintext,omitempty"`
	Ciphertext string `json:"ciphertext,omitempty"`
}

type gcpResponse struct {
	Name       string `json:"name"`
	Ciphertext string `json:"ciphertext"`
	Plaintext  string `json:"plaintext"`
}

// GCPCredentials resolves an OAuth2 transport: a service-account file if set, else Application Default Credentials.
type GCPCredentials struct {
	File string
}

// Find returns an authorising round tripper scoped to Cloud KMS.
func (c GCPCredentials) Find(ctx context.Context) (http.RoundTripper, error) {
	if c.File == "" {
		client, err := google.DefaultClient(ctx, string(GCPScope))
		if err != nil {
			return nil, err
		}
		return client.Transport, nil
	}
	data, err := os.ReadFile(c.File)
	if err != nil {
		return nil, err
	}
	creds, err := google.CredentialsFromJSON(ctx, data, string(GCPScope))
	if err != nil {
		return nil, err
	}
	return &oauth2.Transport{Source: creds.TokenSource}, nil
}
