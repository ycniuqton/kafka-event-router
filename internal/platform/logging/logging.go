// Package logging builds the structured JSON slog logger used by all services.
// Event payloads and secrets must never be logged; the Redactor drops tagged fields.
package logging

import (
	"io"
	"log/slog"

	"github.com/eventrouter/router/internal/platform/config"
)

// Attr names the standard attribute keys every log line may carry.
type Attr string

const (
	AttrService     Attr = "service"
	AttrEnvironment Attr = "env"
	AttrRouteID     Attr = "route_id"
	AttrVersion     Attr = "version"
	AttrError       Attr = "error"
	AttrAddr        Attr = "addr"
	AttrRequestID   Attr = "request_id"
	AttrPrincipal   Attr = "principal"
	AttrMethod      Attr = "method"
	AttrPath        Attr = "path"
	AttrStatus      Attr = "status"
	AttrDurationMS  Attr = "duration_ms"
	AttrListener    Attr = "listener"
	AttrMigration   Attr = "migration"
	AttrTLS         Attr = "tls"
	AttrTraceID     Attr = "trace_id"
	AttrSpanID      Attr = "span_id"
)

// Factory creates loggers bound to a service's identity.
type Factory struct {
	out      io.Writer
	redactor *Redactor
}

// NewFactory returns a Factory writing redacted JSON lines to out.
func NewFactory(out io.Writer) *Factory {
	return &Factory{out: out, redactor: NewRedactor()}
}

// New builds a JSON logger tagged with the service name and environment.
func (f *Factory) New(cfg config.Config) *slog.Logger {
	handler := slog.NewJSONHandler(f.out, &slog.HandlerOptions{Level: cfg.Log.Level, ReplaceAttr: f.redactor.ReplaceAttr})
	return slog.New(handler).With(
		string(AttrService), string(cfg.Service),
		string(AttrEnvironment), string(cfg.Environment),
	)
}
