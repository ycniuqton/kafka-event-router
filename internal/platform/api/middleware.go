package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/metrics"
	"github.com/eventrouter/router/internal/platform/requestid"
)

// RequestIDMiddleware accepts a valid ULID in X-Request-ID or generates one, and echoes it.
type RequestIDMiddleware struct{}

// Handler wraps next.
func (RequestIDMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(string(requestid.HeaderRequestID))
		if _, err := ulid.ParseStrict(id); err != nil {
			id = ulid.Make().String()
		}
		w.Header().Set(string(requestid.HeaderRequestID), id)
		next.ServeHTTP(w, r.WithContext(requestid.With(r.Context(), id)))
	})
}

// RecoverMiddleware turns handler panics into a logged 500 problem.
type RecoverMiddleware struct {
	Responder *Responder
}

// Handler wraps next.
func (m RecoverMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil || rec == http.ErrAbortHandler {
				return
			}
			m.Responder.Problem(w, r, fmt.Errorf("panic: %v", rec))
		}()
		next.ServeHTTP(w, r)
	})
}

// MetricsMiddleware records request counts and latency by route pattern.
type MetricsMiddleware struct {
	Collector *metrics.HTTPCollector
}

// Handler wraps next.
func (m MetricsMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := NewStatusRecorder(w)
		next.ServeHTTP(rec, r)
		m.Collector.Observe(RoutePattern(r), r.Method, rec.Status(), time.Since(start))
	})
}

// LoggingMiddleware writes one structured line per request (never bodies).
type LoggingMiddleware struct {
	Log *slog.Logger
}

// Handler wraps next.
func (m LoggingMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := NewStatusRecorder(w)
		info := &RequestInfo{}
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), requestInfoKey{}, info)))
		m.Log.InfoContext(r.Context(), "http request",
			string(logging.AttrRequestID), RequestIDOf(r),
			string(logging.AttrMethod), r.Method,
			string(logging.AttrPath), RoutePattern(r),
			string(logging.AttrStatus), rec.Status(),
			string(logging.AttrDurationMS), time.Since(start).Milliseconds(),
			string(logging.AttrPrincipal), info.Principal)
	})
}

// NoStoreMiddleware sets Cache-Control: no-store on every response it wraps.
type NoStoreMiddleware struct{}

// Handler wraps next.
func (NoStoreMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(string(HeaderCacheControl), string(CacheNoStore))
		next.ServeHTTP(w, r)
	})
}

// RoutePattern returns the matched chi pattern (bounded label/log cardinality).
func RoutePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		return rc.RoutePattern()
	}
	return ""
}

// StatusRecorder captures the response status code.
type StatusRecorder struct {
	http.ResponseWriter
	status int
}

// NewStatusRecorder wraps w with a default status of 200.
func NewStatusRecorder(w http.ResponseWriter) *StatusRecorder {
	return &StatusRecorder{ResponseWriter: w, status: http.StatusOK}
}

// WriteHeader records the status.
func (s *StatusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Status returns the recorded status.
func (s *StatusRecorder) Status() int { return s.status }

// Unwrap exposes the wrapped writer to http.ResponseController.
func (s *StatusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// RequestInfo collects per-request facts discovered deeper in the chain for the access log.
type RequestInfo struct {
	Principal string
}

type requestInfoKey struct{}

// AnnotatePrincipal records the authenticated principal id for the access log.
func AnnotatePrincipal(r *http.Request, id string) {
	if info, ok := r.Context().Value(requestInfoKey{}).(*RequestInfo); ok {
		info.Principal = id
	}
}
