package codec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RegistryAuthMode selects how the HTTP client authenticates.
type RegistryAuthMode string

const (
	RegistryAuthNone   RegistryAuthMode = "none"
	RegistryAuthBasic  RegistryAuthMode = "basic"
	RegistryAuthBearer RegistryAuthMode = "bearer"
)

// RegistryHTTP holds the wire constants of the Confluent registry REST API.
type RegistryHTTP string

const (
	RegistryContentType   RegistryHTTP = "application/vnd.schemaregistry.v1+json"
	RegistryHeaderAccept  RegistryHTTP = "Accept"
	RegistryHeaderAuth    RegistryHTTP = "Authorization"
	RegistryBearerPrefix  RegistryHTTP = "Bearer "
	RegistryPathSchemaID  RegistryHTTP = "/schemas/ids/%d"
	RegistryPathVersion   RegistryHTTP = "/subjects/%s/versions/%s"
	RegistryLatestVersion RegistryHTTP = "latest"
)

// RegistryDefaultTimeout bounds one registry request.
const RegistryDefaultTimeout = 5 * time.Second

// RegistryMaxBody bounds a registry response body.
const RegistryMaxBody = 8 << 20

// RegistryConfig is the registry setting of one connection.
type RegistryConfig struct {
	URL      string
	Auth     RegistryAuthMode
	Username string
	Password string
	Token    string
	Timeout  time.Duration
}

// RegistryConfigSource is the port to per-connection registry settings (connections#CN-102);
// ok is false when the connection has no registry.
type RegistryConfigSource interface {
	RegistryConfig(ctx context.Context, connectionID string) (cfg RegistryConfig, ok bool, err error)
}

// HTTPRegistryProvider builds one cached HTTP registry client per connection; a changed
// setting replaces the client (and its caches).
type HTTPRegistryProvider struct {
	Source  RegistryConfigSource
	Client  *http.Client
	mu      sync.Mutex
	clients map[string]providerEntry
}

type providerEntry struct {
	cfg RegistryConfig
	reg SchemaRegistry
}

// Registry returns the client of the connection (nil when it has no registry).
func (p *HTTPRegistryProvider) Registry(ctx context.Context, connectionID string) (SchemaRegistry, error) {
	cfg, ok, err := p.Source.RegistryConfig(ctx, connectionID)
	if err != nil || !ok {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.clients == nil {
		p.clients = map[string]providerEntry{}
	}
	if e, hit := p.clients[connectionID]; hit && e.cfg == cfg {
		return e.reg, nil
	}
	reg := NewCachedRegistry(NewHTTPRegistry(cfg, p.Client), DefaultSchemaCacheSize)
	p.clients[connectionID] = providerEntry{cfg: cfg, reg: reg}
	return reg, nil
}

// HTTPRegistry is a Confluent-compatible REST client.
type HTTPRegistry struct {
	cfg    RegistryConfig
	client *http.Client
}

// NewHTTPRegistry builds a client for cfg (nil client = http.DefaultClient).
func NewHTTPRegistry(cfg RegistryConfig, client *http.Client) *HTTPRegistry {
	return &HTTPRegistry{cfg: cfg, client: client}
}

func (r *HTTPRegistry) SchemaByID(ctx context.Context, id int32) (RegisteredSchema, error) {
	s, err := r.get(ctx, fmt.Sprintf(string(RegistryPathSchemaID), id))
	s.ID = id
	return s, err
}

func (r *HTTPRegistry) Latest(ctx context.Context, subject string) (RegisteredSchema, error) {
	return r.get(ctx, fmt.Sprintf(string(RegistryPathVersion), url.PathEscape(subject), RegistryLatestVersion))
}

func (r *HTTPRegistry) Version(ctx context.Context, subject string, version int) (RegisteredSchema, error) {
	return r.get(ctx, fmt.Sprintf(string(RegistryPathVersion), url.PathEscape(subject), strconv.Itoa(version)))
}

// get performs one request and classifies failures: 404 permanent, network and 5xx transient.
func (r *HTTPRegistry) get(ctx context.Context, path string) (RegisteredSchema, error) {
	timeout := r.cfg.Timeout
	if timeout <= 0 {
		timeout = RegistryDefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(r.cfg.URL, "/")+path, nil)
	if err != nil {
		return RegisteredSchema{}, err
	}
	req.Header.Set(string(RegistryHeaderAccept), string(RegistryContentType))
	r.authorize(req)
	resp, err := r.httpClient().Do(req)
	if err != nil {
		return RegisteredSchema{}, fmt.Errorf("%w: schema registry: %v", ErrTransient, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, RegistryMaxBody))
	if err != nil {
		return RegisteredSchema{}, fmt.Errorf("%w: schema registry: %v", ErrTransient, err)
	}
	return RegistryResponse{}.Parse(resp.StatusCode, path, body)
}

func (r *HTTPRegistry) authorize(req *http.Request) {
	switch r.cfg.Auth {
	case RegistryAuthBasic:
		req.SetBasicAuth(r.cfg.Username, r.cfg.Password)
	case RegistryAuthBearer:
		req.Header.Set(string(RegistryHeaderAuth), string(RegistryBearerPrefix)+r.cfg.Token)
	}
}

func (r *HTTPRegistry) httpClient() *http.Client {
	if r.client == nil {
		return http.DefaultClient
	}
	return r.client
}

// RegistryResponse classifies registry HTTP responses.
type RegistryResponse struct{}

// Parse maps status codes to errors and decodes the schema body.
func (RegistryResponse) Parse(status int, path string, body []byte) (RegisteredSchema, error) {
	switch {
	case status == http.StatusNotFound:
		return RegisteredSchema{}, fmt.Errorf("%w: %s", ErrSchemaNotFound, path)
	case status >= http.StatusInternalServerError || status == http.StatusTooManyRequests:
		return RegisteredSchema{}, fmt.Errorf("%w: schema registry returned %d", ErrTransient, status)
	case status != http.StatusOK:
		return RegisteredSchema{}, fmt.Errorf("schema registry returned %d for %s", status, path)
	}
	var s RegisteredSchema
	if err := json.Unmarshal(body, &s); err != nil {
		return RegisteredSchema{}, fmt.Errorf("schema registry response: %v", err)
	}
	return s, nil
}
