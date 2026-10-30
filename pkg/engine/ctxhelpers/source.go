package ctxhelpers

import (
	"context"
	"errors"
	"fmt"
)

// SourceRequestKind is the request shape of a lookup call; it must match the source kind.
type SourceRequestKind string

const (
	RequestHTTP   SourceRequestKind = "http"
	RequestRedis  SourceRequestKind = "redis"
	RequestSQL    SourceRequestKind = "sql"
	RequestStatic SourceRequestKind = "static"
)

// SourceRequest is one rendered lookup request; only the members of its kind are set.
type SourceRequest struct {
	Method    string            `json:"method,omitempty"`
	Path      string            `json:"path,omitempty"`
	Query     map[string]string `json:"query,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      any               `json:"body,omitempty"`
	Op        string            `json:"op,omitempty"`
	Key       string            `json:"key,omitempty"`
	Keys      []string          `json:"keys,omitempty"`
	HashField string            `json:"hashField,omitempty"`
	SQL       string            `json:"sql,omitempty"`
	Params    []any             `json:"params,omitempty"`
}

// SourceCall addresses one request to a lookup source; StepID lets previews answer with mocks,
// TeamID (the route's team) lets the gateway refuse sources of other teams.
type SourceCall struct {
	SourceID string
	StepID   string
	TeamID   string
	Kind     SourceRequestKind
	Request  SourceRequest
}

// AnswerMode tells where an answer came from (shown by the Studio).
type AnswerMode string

const (
	AnswerLive  AnswerMode = "live"
	AnswerMock  AnswerMode = "mock"
	AnswerCache AnswerMode = "cache"
)

// SourceAnswer is the outcome of a call; Found false is a miss (not an error).
type SourceAnswer struct {
	Found bool
	Value any
	Mode  AnswerMode
}

// SourceEvent is an observation reported by lookup steps (cache metrics).
type SourceEvent string

const (
	EventCacheHit  SourceEvent = "hit"
	EventCacheMiss SourceEvent = "miss"
)

// SourceGateway is the PORT of lookup steps to managed lookup sources (implemented by pkg/lookup).
type SourceGateway interface {
	Fetch(ctx context.Context, call SourceCall) (SourceAnswer, error)
	Observe(sourceID string, ev SourceEvent)
}

// ErrSourceUnavailable is the permanent cause when no gateway is wired (tools, API without sources).
var ErrSourceUnavailable = errors.New("lookup sources are not available here")

// SourceError is a failed call; Transient errors (timeout, outage, 5xx/429, circuit open) may heal on retry.
type SourceError struct {
	Transient bool
	Reason    string
	Cause     error
}

func (e *SourceError) Error() string {
	if e.Cause == nil {
		return e.Reason
	}
	return fmt.Sprintf("%s: %v", e.Reason, e.Cause)
}

func (e *SourceError) Unwrap() error { return e.Cause }

// Temporary follows the runtime convention of errors that may heal on retry.
func (e *SourceError) Temporary() bool { return e.Transient }

// SourceErrors classifies call failures.
type SourceErrors struct{}

// IsTransient reports whether err (or its chain) is a transient SourceError or a deadline.
func (SourceErrors) IsTransient(err error) bool {
	var se *SourceError
	if errors.As(err, &se) {
		return se.Transient
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// NoSourceGateway fails every call with ErrSourceUnavailable.
type NoSourceGateway struct{}

func (NoSourceGateway) Fetch(context.Context, SourceCall) (SourceAnswer, error) {
	return SourceAnswer{}, &SourceError{Reason: "no lookup gateway", Cause: ErrSourceUnavailable}
}

func (NoSourceGateway) Observe(string, SourceEvent) {}
