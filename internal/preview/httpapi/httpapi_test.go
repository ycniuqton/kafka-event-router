package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/preview/httpapi"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/kafka"
)

type harness struct {
	srv   *httptest.Server
	codec preview.DirectiveCodec
	gate  *httpapi.RunGate
	now   time.Time
	mask  bool
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	clock := func() time.Time { return h.now }
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	builder, err := preview.NewPipelineBuilder(compile.NewDefaultCompiler(), preview.NoScripts{}, ctxhelpers.StaticSaltProvider{})
	if err != nil {
		t.Fatal(err)
	}
	clients, _ := preview.NewSamplingClientRegistry().Build(preview.SamplerUnavailable)
	decoder := preview.NewSampleDecoder(nil)
	svc := preview.NewService(preview.ServiceDeps{
		Sources: preview.NewSampleSourceManager(
			preview.KafkaLatestSource{Sampling: preview.KafkaSampling{Clients: clients}, Planner: kafka.LatestOffsetPlanner{}},
			preview.UploadSource{Clock: clock}),
		Sessions: preview.NewSessionStoreManager(preview.NewMemorySessionStore(clock), clock),
		Runner:   preview.PreviewRunner{Builder: builder, Differ: preview.StructuralDiffer{}, Deadline: 5 * time.Second, Now: clock},
		Views:    preview.SampleViewBuilder{Decoder: decoder}, Schemas: preview.SchemaBuilder{Decoder: decoder, Inferrer: preview.NewSchemaInferrer()},
		Metrics: preview.NewMetrics(nil), Log: log, Now: clock, SchemaJobs: 1,
	})
	responder := api.NewResponder(preview.NewProblemCatalog(), log)
	h.codec = preview.NewDirectiveCodec("s", clock)
	h.gate = httpapi.NewRunGate(1)
	m := api.NewMounter(responder, []api.GlobalMiddleware{api.RequestIDMiddleware{}}, []api.RouteMiddleware{httpapi.DirectiveStage{Codec: h.codec, Responder: responder}})
	r := chi.NewRouter()
	m.Install(r)
	m.Mount(r, httpapi.NewModule(httpapi.Deps{Service: svc, Responder: responder, Gate: h.gate}))
	h.srv = httptest.NewServer(r)
	t.Cleanup(h.srv.Close)
	return h
}

func (h *harness) do(t *testing.T, user, method string, e preview.Endpoint, id, query string, body any) (int, map[string]any) {
	t.Helper()
	path := preview.PrefixInternal.Pattern(e)
	path = string(bytes.Replace([]byte(path), []byte("{id}"), []byte(id), 1)) + query
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, rd)
	if user != "" {
		v, _ := h.codec.Encode(preview.Caller{UserID: user, TeamID: "t1", Mask: h.mask})
		req.Header.Set(string(preview.HeaderCaller), v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func route(t *testing.T) json.RawMessage {
	raw, err := os.ReadFile("../testdata/golden/filter_only/route.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

const upload = "{\"eventType\":\"OrderPaid\",\"amount\":150}\n{\"eventType\":\"OrderCreated\",\"amount\":500}\n{\"eventType\":\"OrderPaid\",\"amount\":50}\n"

func TestUploadRunPageSchemaDeleteFlow(t *testing.T) {
	h := newHarness(t)
	status, body := h.do(t, "u1", http.MethodPost, preview.EndpointSamples, "", "", map[string]any{"mode": "upload", "upload": upload})
	if status != http.StatusCreated {
		t.Fatalf("load = %d %v", status, body)
	}
	session := body["session"].(map[string]any)
	id := session["id"].(string)
	if session["loaded"].(float64) != 3 || len(body["events"].([]any)) != 3 {
		t.Fatalf("load body = %v", body)
	}
	status, body = h.do(t, "u1", http.MethodPost, preview.EndpointRun, "", "", map[string]any{"sessionId": id, "config": route(t)})
	summary, _ := body["summary"].(map[string]any)
	if status != http.StatusOK || summary["passed"].(float64) != 1 || summary["filtered"].(float64) != 2 {
		t.Fatalf("run = %d %v", status, body)
	}
	status, body = h.do(t, "u1", http.MethodGet, preview.EndpointSessionEvents, id, "?limit=2&q=orderpaid", nil)
	if status != http.StatusOK || len(body["events"].([]any)) != 2 {
		t.Fatalf("events = %d %v", status, body)
	}
	status, body = h.do(t, "u1", http.MethodGet, preview.EndpointSessionSchema, id, "", nil)
	if status != http.StatusOK || body["samples"].(float64) != 3 {
		t.Fatalf("schema = %d %v", status, body)
	}
	if status, _ = h.do(t, "u1", http.MethodDelete, preview.EndpointSession, id, "", nil); status != http.StatusNoContent {
		t.Fatalf("delete = %d", status)
	}
	if status, body = h.do(t, "u1", http.MethodPost, preview.EndpointRun, "", "", map[string]any{"sessionId": id, "config": route(t)}); status != http.StatusGone {
		t.Fatalf("run after delete = %d %v", status, body)
	}
}

func TestProblems(t *testing.T) {
	h := newHarness(t)
	_, body := h.do(t, "u1", http.MethodPost, preview.EndpointSamples, "", "", map[string]any{"mode": "upload", "upload": upload})
	id := body["session"].(map[string]any)["id"].(string)
	cases := []struct {
		name   string
		user   string
		method string
		e      preview.Endpoint
		id     string
		body   any
		status int
		path   string
	}{
		{name: "no directive", method: http.MethodGet, e: preview.EndpointSessionSchema, id: id, status: http.StatusUnauthorized},
		{name: "foreign session", user: "u2", method: http.MethodGet, e: preview.EndpointSessionSchema, id: id, status: http.StatusNotFound},
		{name: "unknown mode", user: "u1", method: http.MethodPost, e: preview.EndpointSamples, body: map[string]any{"mode": "bogus"}, status: http.StatusBadRequest, path: "/mode"},
		{name: "kafka sampler missing", user: "u1", method: http.MethodPost, e: preview.EndpointSamples, body: map[string]any{"mode": "latest", "connectionId": "c", "topics": []string{"t"}}, status: http.StatusBadGateway},
		{name: "invalid config", user: "u1", method: http.MethodPost, e: preview.EndpointRun, body: map[string]any{"sessionId": id, "config": map[string]any{"apiVersion": "router/v1"}}, status: http.StatusUnprocessableEntity},
		{name: "no input", user: "u1", method: http.MethodPost, e: preview.EndpointRun, body: map[string]any{"config": route(t)}, status: http.StatusBadRequest, path: "/sessionId"},
		{name: "inline run", user: "u2", method: http.MethodPost, e: preview.EndpointTestRuns, body: map[string]any{"config": route(t), "events": []map[string]any{{"value": map[string]any{"eventType": "OrderPaid", "amount": 101}}}}, status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := h.do(t, tc.user, tc.method, tc.e, tc.id, "", tc.body)
			if status != tc.status || (tc.path != "" && body["path"] != tc.path) {
				t.Fatalf("status %d body %v", status, body)
			}
		})
	}
}

func TestRunGateBusy(t *testing.T) {
	h := newHarness(t)
	if !h.gate.TryAcquire() {
		t.Fatal("gate should start empty")
	}
	defer h.gate.Release()
	status, body := h.do(t, "u1", http.MethodPost, preview.EndpointRun, "", "", map[string]any{"config": route(t), "events": []map[string]any{{"value": 1}}})
	if status != http.StatusTooManyRequests || body["type"] != string(preview.TypeBusy) {
		t.Fatalf("status %d body %v", status, body)
	}
}

// TestSessionSourceAndInlineRunsCopiedFromASession: GET /sessions/{id} names the sample source, and inline runs
// naming their sample session keep that session's masking decision.
func TestSessionSourceAndInlineRunsCopiedFromASession(t *testing.T) {
	h := newHarness(t)
	h.mask = true
	_, body := h.do(t, "u1", http.MethodPost, preview.EndpointSamples, "", "", map[string]any{"mode": "upload", "upload": upload})
	id := body["session"].(map[string]any)["id"].(string)
	h.mask = false
	status, src := h.do(t, "u1", http.MethodGet, preview.EndpointSession, id, "", nil)
	if status != http.StatusOK || src["id"] != id || src["mode"] != "upload" || src["masked"] != true || src["connectionId"] != nil {
		t.Fatalf("source = %d %v", status, src)
	}
	if status, _ := h.do(t, "u2", http.MethodGet, preview.EndpointSession, id, "", nil); status != http.StatusNotFound {
		t.Fatalf("foreign source = %d", status)
	}
	inline := func(sampleSession string) map[string]any {
		return map[string]any{"config": route(t), "sampleSessionId": sampleSession, "events": []map[string]any{{"value": map[string]any{"eventType": "OrderPaid", "amount": 101}}}}
	}
	if status, run := h.do(t, "u1", http.MethodPost, preview.EndpointRun, "", "", inline(id)); status != http.StatusOK || run["masked"] != true {
		t.Fatalf("inline run copied from a masked session = %d %v", status, run)
	}
	if status, run := h.do(t, "u1", http.MethodPost, preview.EndpointRun, "", "", inline("")); status != http.StatusOK || run["masked"] == true {
		t.Fatalf("plain inline run = %d %v", status, run)
	}
	if status, _ := h.do(t, "u2", http.MethodPost, preview.EndpointRun, "", "", inline(id)); status != http.StatusNotFound {
		t.Fatalf("inline run naming a foreign session = %d", status)
	}
}
