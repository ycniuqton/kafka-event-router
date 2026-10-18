package tracing

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/config"
)

func TestMiddlewareExportsNamedSpanAndLogsTraceID(t *testing.T) {
	var spans, logs bytes.Buffer
	reg := NewRegistry()
	reg.stdout = &spans
	cfg := config.Config{Service: config.ServiceAPI, Tracing: config.TracingConfig{Exporter: config.TracingStdout, SampleRatio: 1}}
	provider, err := reg.Setup(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(NewLogHandler(slog.NewJSONHandler(&logs, nil)))
	var traceID string
	r := chi.NewRouter()
	r.Use(Middleware{}.Handler)
	r.Get("/routes/{id}", func(w http.ResponseWriter, r *http.Request) {
		traceID = Correlation{}.TraceID(r.Context())
		log.InfoContext(r.Context(), "inside")
		w.WriteHeader(http.StatusTeapot)
	})
	req := httptest.NewRequest("GET", "/routes/42", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if traceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("incoming traceparent not joined: %q", traceID)
	}
	if !strings.Contains(spans.String(), `"Name":"GET /routes/{id}"`) || !strings.Contains(spans.String(), "event-router-api") {
		t.Fatalf("span not exported as expected: %s", spans.String())
	}
	if !strings.Contains(logs.String(), `"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"`) {
		t.Fatalf("log lacks trace id: %s", logs.String())
	}
}

func TestNoneExporterAndUnknownKind(t *testing.T) {
	p, err := NewRegistry().Setup(context.Background(), config.Config{Tracing: config.TracingConfig{Exporter: config.TracingNone}})
	if err != nil || p.Shutdown(context.Background()) != nil {
		t.Fatalf("none exporter: %v", err)
	}
	if _, err := NewRegistry().Setup(context.Background(), config.Config{Tracing: config.TracingConfig{Exporter: "zipkin"}}); err == nil {
		t.Fatal("unknown exporter must fail")
	}
}
