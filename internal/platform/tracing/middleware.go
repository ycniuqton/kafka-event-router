package tracing

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/eventrouter/router/internal/platform/logging"
)

// InstrumentationName is the tracer scope of the HTTP server middleware.
type InstrumentationName string

const HTTPServerScope InstrumentationName = "github.com/eventrouter/router/internal/platform/tracing/http"

// SpanAttr names span attributes (OTel HTTP semantic conventions).
type SpanAttr string

const (
	AttrHTTPMethod SpanAttr = "http.request.method"
	AttrHTTPRoute  SpanAttr = "http.route"
	AttrHTTPStatus SpanAttr = "http.response.status_code"
	AttrURLPath    SpanAttr = "url.path"
)

// StatusServerError is the first status marking a span as failed.
type StatusServerError int

const ServerErrorMin StatusServerError = 500

// Middleware starts one server span per request (joining an incoming traceparent) named "METHOD /route/{pattern}".
type Middleware struct{}

// Handler wraps next.
func (Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := otel.Tracer(string(HTTPServerScope)).Start(ctx, r.Method, trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String(string(AttrHTTPMethod), r.Method), attribute.String(string(AttrURLPath), r.URL.Path)))
		defer span.End()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r.WithContext(ctx))
		route := ""
		if rc := chi.RouteContext(ctx); rc != nil {
			route = rc.RoutePattern()
		}
		span.SetName(r.Method + " " + route)
		span.SetAttributes(attribute.String(string(AttrHTTPRoute), route), attribute.Int(string(AttrHTTPStatus), rec.status))
		if rec.status >= int(ServerErrorMin) {
			span.SetStatus(codes.Error, http.StatusText(rec.status))
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the wrapped writer (http.ResponseController, WebSocket hijack).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// LogHandler adds trace_id and span_id to every record logged with a context carrying a recording span.
type LogHandler struct {
	slog.Handler
}

// NewLogHandler wraps h.
func NewLogHandler(h slog.Handler) *LogHandler {
	return &LogHandler{Handler: h}
}

// Handle implements slog.Handler.
func (h *LogHandler) Handle(ctx context.Context, rec slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		rec.AddAttrs(slog.String(string(logging.AttrTraceID), sc.TraceID().String()), slog.String(string(logging.AttrSpanID), sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, rec)
}

// WithAttrs implements slog.Handler.
func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup implements slog.Handler.
func (h *LogHandler) WithGroup(name string) slog.Handler {
	return &LogHandler{Handler: h.Handler.WithGroup(name)}
}

// Correlation reads trace identifiers from a context.
type Correlation struct{}

// TraceID returns the trace id of ctx's span, or "".
func (Correlation) TraceID(ctx context.Context) string {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		return sc.TraceID().String()
	}
	return ""
}
