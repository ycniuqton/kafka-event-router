package schemaregistry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/eventrouter/router/pkg/kafka"
)

// Endpoint names the registry REST paths and media types used here.
type Endpoint string

const (
	PathSubjects  Endpoint = "/subjects"
	PathSeparator Endpoint = "/"
	HeaderAccept  Endpoint = "Accept"
	MediaTypeJSON Endpoint = "application/vnd.schemaregistry.v1+json, application/json"
)

// ClientLimits bounds registry requests.
type ClientLimits int64

const (
	RequestTimeout ClientLimits = ClientLimits(10 * time.Second)
	MaxDrainBytes  ClientLimits = 64 << 10
)

// ErrNotConfigured is returned when a connection has no schema registry.
var ErrNotConfigured = errors.New("schemaregistry: no schema registry configured")

// StatusError is a non-2xx registry answer.
type StatusError struct {
	Code int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("schemaregistry: unexpected status %d", e.Code)
}

// StatusReasons maps registry HTTP statuses to failure reasons (others: unknown).
type StatusReasons map[int]kafka.FailureReason

// DefaultStatusReasons is the domain table of registry answers.
var DefaultStatusReasons = StatusReasons{
	http.StatusUnauthorized:       kafka.ReasonAuthFailed,
	http.StatusForbidden:          kafka.ReasonNotAuthorized,
	http.StatusNotFound:           kafka.ReasonInvalidConfig,
	http.StatusBadGateway:         kafka.ReasonUnreachable,
	http.StatusServiceUnavailable: kafka.ReasonUnreachable,
	http.StatusGatewayTimeout:     kafka.ReasonTimeout,
}

// ReasonOf returns the reason of a StatusError, or false for other errors (classified elsewhere).
func (t StatusReasons) ReasonOf(err error) (kafka.FailureReason, bool) {
	var se *StatusError
	if !errors.As(err, &se) {
		return "", false
	}
	if reason, ok := t[se.Code]; ok {
		return reason, true
	}
	return kafka.ReasonUnknown, true
}

// Client sends authenticated requests to the registry of a connection spec.
type Client struct {
	http  *http.Client
	auths *AuthRegistry
}

// NewClient builds a client; a nil http client gets the default request timeout.
func NewClient(httpClient *http.Client, auths *AuthRegistry) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: time.Duration(RequestTimeout)}
	}
	return &Client{http: httpClient, auths: auths}
}

// Ping lists subjects: success proves the registry is reachable and accepts the credentials.
func (c *Client) Ping(ctx context.Context, spec kafka.ConnectionSpec) error {
	if spec.SchemaRegistry == nil {
		return ErrNotConfigured
	}
	auth, err := c.auths.For(spec.SchemaRegistry.Auth.ModeOrDefault())
	if err != nil {
		return err
	}
	target := strings.TrimRight(spec.SchemaRegistry.URL, string(PathSeparator)) + string(PathSubjects)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set(string(HeaderAccept), string(MediaTypeJSON))
	auth.Apply(req, spec)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, int64(MaxDrainBytes)))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &StatusError{Code: resp.StatusCode}
	}
	return nil
}
