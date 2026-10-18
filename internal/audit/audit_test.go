package audit

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eventrouter/router/internal/platform/logging"
)

type memRecorder struct{ entries []Entry }

func (m *memRecorder) Record(_ context.Context, e Entry) error {
	m.entries = append(m.entries, e)
	return nil
}

func TestDiffBuilderMergePatchWithRedaction(t *testing.T) {
	b := NewDiffBuilder(logging.NewRedactor())
	before := map[string]any{"name": "a", "tags": []string{"x"}, "config": map[string]any{"retries": 1, "password": "old"}, "gone": true}
	after := map[string]any{"name": "b", "tags": []string{"x"}, "config": map[string]any{"retries": 2, "password": "new"}}
	raw, err := b.Build(before, after)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	cfg, _ := got["config"].(map[string]any)
	if got["name"] != "b" || got["gone"] != nil || len(got) != 3 || cfg["retries"] != float64(2) || cfg["password"] != "[REDACTED]" {
		t.Fatalf("diff = %s", raw)
	}
	if _, ok := got["tags"]; ok {
		t.Fatalf("unchanged field in diff: %s", raw)
	}
	created, _ := b.Build(nil, map[string]any{"client_secret": "s", "name": "n"})
	if string(created) != `{"client_secret":"[REDACTED]","name":"n"}` {
		t.Fatalf("create diff = %s", created)
	}
}

func source(*http.Request) RequestContext {
	return RequestContext{Actor: Actor{Kind: ActorUser}, RequestID: "r1", IP: "10.0.0.1"}
}

func serve(m *Middleware, h http.HandlerFunc) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	m.Wrap("route.update", h).ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/api/v1/routes/1", nil))
	return w
}

func TestMiddlewareScopeAndFallback(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rec := &memRecorder{}
	m := NewMiddleware(rec, NewDiffBuilder(logging.NewRedactor()), source, log)
	serve(m, func(w http.ResponseWriter, r *http.Request) {
		scope, _ := FromContext(r.Context())
		_ = scope.Record(r.Context(), Change{Action: "route.update", Object: ObjectRoute, ObjectID: "1", Before: map[string]int{"a": 1}, After: map[string]int{"a": 2}})
	})
	if len(rec.entries) != 1 || rec.entries[0].RequestID != "r1" || string(rec.entries[0].Diff) != `{"a":2}` {
		t.Fatalf("scoped: %+v", rec.entries)
	}
	serve(m, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	if len(rec.entries) != 2 || rec.entries[1].Object != ObjectRequest || rec.entries[1].Action != "route.update" {
		t.Fatalf("fallback: %+v", rec.entries)
	}
	serve(m, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusConflict) })
	if len(rec.entries) != 2 {
		t.Fatalf("failed request recorded: %+v", rec.entries)
	}
}

func TestMiddlewareStrictModePanics(t *testing.T) {
	m := NewMiddleware(&memRecorder{}, nil, source, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.StrictMode = true
	defer func() {
		if recover() == nil {
			t.Fatal("StrictMode must panic on unrecorded mutation")
		}
	}()
	serve(m, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
}
