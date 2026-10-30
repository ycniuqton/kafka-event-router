package lookup

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// HTTPLimit bounds HTTP answers and connections.
type HTTPLimit int64

const (
	MaxAnswerBytes   HTTPLimit = 1 << 20
	IdleConnsPerHost HTTPLimit = 32
)

// HTTPHeader names headers set by the HTTP client.
type HTTPHeader string

const (
	HeaderAuthorization HTTPHeader = "Authorization"
	HeaderContentType   HTTPHeader = "Content-Type"
	HeaderAccept        HTTPHeader = "Accept"
	BearerPrefix        HTTPHeader = "Bearer "
	MediaJSON           HTTPHeader = "application/json"
)

// ErrHTTPStatus is the cause of non-2xx answers.
var ErrHTTPStatus = errors.New("unexpected HTTP status")

// ErrBadCA is returned for an unparsable CA PEM.
var ErrBadCA = errors.New("CA certificate is not valid PEM")

// AuthApplier sets the credentials of one HTTP auth mode on a request.
type AuthApplier interface {
	Mode() HTTPAuthMode
	Apply(r *http.Request, auth HTTPAuth, secrets SecretBundle)
	Secrets() []SecretField
}

// NoAuth sends no credentials.
type NoAuth struct{}

func (NoAuth) Mode() HTTPAuthMode                          { return AuthNone }
func (NoAuth) Apply(*http.Request, HTTPAuth, SecretBundle) {}
func (NoAuth) Secrets() []SecretField                      { return nil }

// BearerAuth sends `Authorization: Bearer <token>`.
type BearerAuth struct{}

func (BearerAuth) Mode() HTTPAuthMode { return AuthBearer }
func (BearerAuth) Apply(r *http.Request, _ HTTPAuth, s SecretBundle) {
	r.Header.Set(string(HeaderAuthorization), string(BearerPrefix)+s.Token)
}
func (BearerAuth) Secrets() []SecretField { return []SecretField{SecretToken} }

// BasicAuth sends HTTP basic credentials.
type BasicAuth struct{}

func (BasicAuth) Mode() HTTPAuthMode { return AuthBasic }
func (BasicAuth) Apply(r *http.Request, a HTTPAuth, s SecretBundle) {
	r.SetBasicAuth(a.Username, s.Password)
}
func (BasicAuth) Secrets() []SecretField { return []SecretField{SecretPassword} }

// HeaderAuth sends a custom header (`X-Api-Key: …`).
type HeaderAuth struct{}

func (HeaderAuth) Mode() HTTPAuthMode { return AuthHeader }
func (HeaderAuth) Apply(r *http.Request, a HTTPAuth, s SecretBundle) {
	r.Header.Set(a.HeaderName, s.HeaderValue)
}
func (HeaderAuth) Secrets() []SecretField { return []SecretField{SecretHeaderValue} }

// AuthAppliers selects the applier of an auth mode.
var AuthAppliers = map[HTTPAuthMode]AuthApplier{AuthNone: NoAuth{}, AuthBearer: BearerAuth{}, AuthBasic: BasicAuth{}, AuthHeader: HeaderAuth{}}

// HTTPFactory builds HTTP clients.
type HTTPFactory struct{}

func (HTTPFactory) Kind() SourceKind { return KindHTTP }

func (HTTPFactory) New(spec SourceSpec) (Client, error) {
	if spec.HTTP == nil {
		return nil, fmt.Errorf("http settings missing")
	}
	tlsCfg, err := TLSBuilder{}.Build(spec.HTTP.TLS)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{TLSClientConfig: tlsCfg, MaxConnsPerHost: spec.Limits.MaxConcurrency, MaxIdleConnsPerHost: int(IdleConnsPerHost), IdleConnTimeout: time.Minute}
	auth, ok := AuthAppliers[spec.HTTP.Auth.Mode]
	if !ok {
		auth = NoAuth{}
	}
	return &HTTPClient{settings: *spec.HTTP, secrets: spec.Secrets, auth: auth, http: &http.Client{Transport: transport}}, nil
}

// TLSBuilder turns TLSSettings into a tls.Config.
type TLSBuilder struct{}

// Build returns a TLS 1.2+ config with the optional CA pool.
func (TLSBuilder) Build(s TLSSettings) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: s.InsecureSkipVerify}
	if s.CAPEM == "" {
		return cfg, nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(s.CAPEM)) {
		return nil, ErrBadCA
	}
	cfg.RootCAs = pool
	return cfg, nil
}

// HTTPClient calls an HTTP source: 2xx → found, 404 → miss, 429/5xx → transient, other → permanent.
type HTTPClient struct {
	settings HTTPSettings
	secrets  SecretBundle
	auth     AuthApplier
	http     *http.Client
}

// Fetch implements Client.
func (c *HTTPClient) Fetch(ctx context.Context, req ctxhelpers.SourceRequest) (ctxhelpers.SourceAnswer, error) {
	r, err := c.request(ctx, req)
	if err != nil {
		return ctxhelpers.SourceAnswer{}, Failures{}.Permanent(ReasonRejected, err)
	}
	resp, err := c.http.Do(r)
	if err != nil {
		return ctxhelpers.SourceAnswer{}, Failures{}.Network(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(MaxAnswerBytes)))
	if err != nil {
		return ctxhelpers.SourceAnswer{}, Failures{}.Network(err)
	}
	return HTTPStatuses{}.Answer(resp.StatusCode, body)
}

// request builds the outgoing request: base URL + path + query, default and request headers, auth, JSON body.
func (c *HTTPClient) request(ctx context.Context, req ctxhelpers.SourceRequest) (*http.Request, error) {
	target, err := HTTPTarget{}.URL(c.settings.BaseURL, req.Path, req.Query)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if req.Body != nil {
		raw, err := json.Marshal(req.Body)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	r, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	c.headers(r, req, body != nil)
	return r, nil
}

func (c *HTTPClient) headers(r *http.Request, req ctxhelpers.SourceRequest, hasBody bool) {
	r.Header.Set(string(HeaderAccept), string(MediaJSON))
	if hasBody {
		r.Header.Set(string(HeaderContentType), string(MediaJSON))
	}
	for k, v := range c.settings.Headers {
		r.Header.Set(k, v)
	}
	for k, v := range req.Headers {
		r.Header.Set(k, v)
	}
	c.auth.Apply(r, c.settings.Auth, c.secrets)
}

// Probe GETs the base URL: transport errors fail reachable, 401/403 fail authenticated, 5xx fail sample.
func (c *HTTPClient) Probe(ctx context.Context) error {
	_, err := c.Fetch(ctx, ctxhelpers.SourceRequest{Method: http.MethodGet})
	return HTTPProbe{}.Classify(err)
}

// Close implements Client.
func (c *HTTPClient) Close() { c.http.CloseIdleConnections() }

// HTTPTarget joins base URL, path and query.
type HTTPTarget struct{}

// URL returns the request URL.
func (HTTPTarget) URL(base, path string, query map[string]string) (string, error) {
	u, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		return "", err
	}
	if len(query) > 0 {
		q := u.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

// HTTPStatuses maps answer statuses to outcomes.
type HTTPStatuses struct{}

// Answer classifies one response.
func (HTTPStatuses) Answer(status int, body []byte) (ctxhelpers.SourceAnswer, error) {
	switch {
	case status == http.StatusNotFound:
		return ctxhelpers.SourceAnswer{Mode: ctxhelpers.AnswerLive}, nil
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return ctxhelpers.SourceAnswer{Found: true, Value: AnswerDecoder{}.JSONOrText(body), Mode: ctxhelpers.AnswerLive}, nil
	case status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return ctxhelpers.SourceAnswer{}, Failures{}.Transient(ReasonUnavailable, StatusCause{Code: status})
	}
	return ctxhelpers.SourceAnswer{}, Failures{}.Permanent(ReasonRejected, StatusCause{Code: status})
}

// StatusCause is an HTTP status as an error cause.
type StatusCause struct {
	Code int
}

func (s StatusCause) Error() string {
	return fmt.Sprintf("%v %d %s", ErrHTTPStatus, s.Code, http.StatusText(s.Code))
}

func (s StatusCause) Unwrap() error { return ErrHTTPStatus }

// HTTPProbe maps probe outcomes to checklist stages.
type HTTPProbe struct{}

// Classify returns nil or a ProbeError.
func (HTTPProbe) Classify(err error) error {
	if err == nil {
		return nil
	}
	var sc StatusCause
	if errors.As(err, &sc) {
		if sc.Code == http.StatusUnauthorized || sc.Code == http.StatusForbidden {
			return &ProbeError{Stage: StageAuthenticated, Hint: string(HintCredentials), Cause: err}
		}
		if sc.Code < http.StatusInternalServerError {
			return nil
		}
		return &ProbeError{Stage: StageSample, Hint: string(HintServerError), Cause: err}
	}
	return &ProbeError{Stage: StageReachable, Hint: string(HintReachable), Cause: err}
}

// AnswerDecoder turns raw answers into engine values.
type AnswerDecoder struct{}

// JSONOrText decodes JSON with the engine number policy; anything else is returned as text.
func (AnswerDecoder) JSONOrText(b []byte) any {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil || dec.More() {
		return string(b)
	}
	return codec.NumberPolicy{}.Normalize(v)
}
