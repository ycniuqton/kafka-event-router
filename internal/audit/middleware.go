package audit

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/eventrouter/router/internal/platform/logging"
)

// RequestContextSource resolves who made the request (wired by the api package from auth).
type RequestContextSource func(r *http.Request) RequestContext

// LogEvent names audit-related warning events.
type LogEvent string

const EventMissingScope LogEvent = "audit.missing_scope"

// StatusBound delimits the 2xx success range [min, max).
type StatusBound int

const (
	StatusSuccessMin StatusBound = 200
	StatusSuccessMax StatusBound = 300
)

// Middleware installs a Scope on mutating requests and records a fallback entry when a
// successful mutation recorded nothing. StrictMode (tests) turns the fallback into a panic.
type Middleware struct {
	recorder   Recorder
	diff       *DiffBuilder
	source     RequestContextSource
	log        *slog.Logger
	StrictMode bool
}

// NewMiddleware builds the audit middleware.
func NewMiddleware(recorder Recorder, diff *DiffBuilder, source RequestContextSource, log *slog.Logger) *Middleware {
	return &Middleware{recorder: recorder, diff: diff, source: source, log: log}
}

// Wrap guards next for a mutating route audited as action.
func (m *Middleware) Wrap(action Action, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := m.source(r)
		scope := NewScope(rc, m.recorder, m.diff)
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r.WithContext(WithScope(r.Context(), scope)))
		if sw.status < int(StatusSuccessMin) || sw.status >= int(StatusSuccessMax) || scope.Recorded() > 0 {
			return
		}
		m.fallback(r, rc, action)
	})
}

func (m *Middleware) fallback(r *http.Request, rc RequestContext, action Action) {
	if m.StrictMode {
		panic(fmt.Sprintf("audit: %s %s mutated without recording an audit entry", r.Method, r.URL.Path))
	}
	m.log.Warn(string(EventMissingScope), string(logging.AttrPath), r.URL.Path, string(logging.AttrRequestID), rc.RequestID)
	entry := rc.Apply(Entry{Action: action, Object: ObjectRequest, ObjectID: r.URL.Path})
	if err := m.recorder.Record(r.Context(), entry); err != nil {
		m.log.Error("audit fallback failed", string(logging.AttrError), err)
	}
}

// statusWriter captures the response status for the success check.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
