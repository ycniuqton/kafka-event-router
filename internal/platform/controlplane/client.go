// Package controlplane is the HTTP client workers and preview use to reach the API's internal listener
// (secrets today; config and leases later). Feature folders must not build their own clients.
package controlplane

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/eventrouter/router/internal/platform/config"
)

// Path names internal API paths the client calls.
type Path string

const PathSecrets Path = "/internal/v1/secrets/"

// AuthScheme is the Authorization header scheme.
type AuthScheme string

const SchemeBearer AuthScheme = "Bearer "

// HeaderAuthorization names the credential header.
type Header string

const HeaderAuthorization Header = "Authorization"

// RequestTimeout bounds one control-plane call.
type RequestTimeout time.Duration

const DefaultRequestTimeout RequestTimeout = RequestTimeout(10 * time.Second)

// Credential supplies the workload credential for each call.
type Credential interface {
	Token(ctx context.Context) (string, error)
}

// Client calls the API internal listener with a workload credential and optional CA pinning.
type Client struct {
	baseURL    string
	http       *http.Client
	credential Credential
}

// NewClient builds a client from configuration using the credential registry.
func NewClient(cp config.ControlPlaneConfig, kind config.WorkloadAuthKind) (*Client, error) {
	cred, err := NewCredentialRegistry().Build(kind, cp)
	if err != nil {
		return nil, err
	}
	httpClient, err := TransportFactory{}.ClientFor(cp)
	if err != nil {
		return nil, err
	}
	return &Client{baseURL: strings.TrimRight(cp.InternalURL, "/"), http: httpClient, credential: cred}, nil
}

// NewClientWith builds a client from explicit parts (tests).
func NewClientWith(baseURL string, httpClient *http.Client, cred Credential) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient, credential: cred}
}

// GetSecret fetches and decodes a secret by reference.
func (c *Client) GetSecret(ctx context.Context, ref string) ([]byte, error) {
	var body struct {
		ValueB64 string `json:"value_b64"`
	}
	if err := c.get(ctx, string(PathSecrets)+url.PathEscape(ref), &body); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(body.ValueB64)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	token, err := c.credential.Token(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set(string(HeaderAuthorization), string(SchemeBearer)+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("controlplane: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &StatusError{Status: resp.StatusCode, Path: path}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// StatusError reports a non-200 control-plane response.
type StatusError struct {
	Status int
	Path   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("controlplane: %s returned %d", e.Path, e.Status)
}

// TransportFactory builds HTTP clients trusting an optional pinned CA.
type TransportFactory struct{}

// Client returns a TLS 1.2+ client; caFile pins the control-plane CA when set.
func (f TransportFactory) Client(caFile string) (*http.Client, error) {
	return f.ClientFor(config.ControlPlaneConfig{CAFile: caFile})
}

// ClientFor returns a TLS 1.2+ client pinning cp.CAFile and presenting cp's client certificate (mtls) when set.
func (TransportFactory) ClientFor(cp config.ControlPlaneConfig) (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cp.CAFile != "" {
		pem, err := os.ReadFile(cp.CAFile)
		if err != nil {
			return nil, fmt.Errorf("controlplane: read CA: %w", err)
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(pem)
		tlsCfg.RootCAs = pool
	}
	if cp.ClientCertFile != "" {
		pair, err := tls.LoadX509KeyPair(cp.ClientCertFile, cp.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("controlplane: load client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{pair}
	}
	return &http.Client{Timeout: time.Duration(DefaultRequestTimeout), Transport: &http.Transport{TLSClientConfig: tlsCfg}}, nil
}
