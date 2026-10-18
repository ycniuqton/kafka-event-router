package assisthttp_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/assist/assembly"
	"github.com/eventrouter/router/internal/assist/mock"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
)

func server(t *testing.T, rate int) *httptest.Server {
	t.Helper()
	reg := assist.NewProviderRegistry()
	mock.Register(reg)
	responder := api.NewResponder(problem.NewCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	settings := assist.Settings{Provider: assist.ProviderMock, RatePerMinute: rate, Limits: assist.DefaultLimits()}
	c, err := assembly.Build(assembly.Deps{Settings: settings, Providers: reg, Responder: responder, Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	m := api.NewMounter(responder, nil, nil)
	m.Install(r)
	m.Mount(r, c.Module)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func post(t *testing.T, srv *httptest.Server, path, body string) (int, map[string]any, http.Header) {
	t.Helper()
	res, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out, res.Header
}

func TestProposeEndpoint(t *testing.T) {
	srv := server(t, 30)
	status, body, _ := post(t, srv, "/api/v1/assist/steps",
		`{"instruction":"hide the email","currentSteps":[],"fields":["value.customer.email"],"routeId":"r1"}`)
	if status != http.StatusOK {
		t.Fatalf("status %d %v", status, body)
	}
	steps := body["steps"].([]any)
	added := body["diff"].(map[string]any)["added"].([]any)
	if len(steps) != 1 || added[0] != "Mask customer.email (email style)" || body["provider"] != "mock" || body["proposalId"] == "" {
		t.Fatalf("unexpected body %v", body)
	}
}

func TestProposeValidationAndRateLimit(t *testing.T) {
	srv := server(t, 1)
	status, body, _ := post(t, srv, "/api/v1/assist/steps", `{"instruction":" "}`)
	if status != http.StatusUnprocessableEntity || body["path"] != "/instruction" {
		t.Fatalf("empty instruction: %d %v", status, body)
	}
	if status, _, _ = post(t, srv, "/api/v1/assist/steps", `{"instruction":"mask a"}`); status != http.StatusOK {
		t.Fatalf("first call %d", status)
	}
	status, _, hdr := post(t, srv, "/api/v1/assist/steps", `{"instruction":"mask a"}`)
	if status != http.StatusTooManyRequests || hdr.Get("Retry-After") == "" {
		t.Fatalf("rate limit: %d %v", status, hdr)
	}
}

func TestExplainEndpoint(t *testing.T) {
	srv := server(t, 30)
	status, body, _ := post(t, srv, "/api/v1/assist/explain", `{"step":{"kind":"js","code":"function transform(event, ctx) { return null; }"}}`)
	if status != http.StatusOK || body["provider"] != "mock" || len(body["bullets"].([]any)) != 1 {
		t.Fatalf("js explain: %d %v", status, body)
	}
	status, body, _ = post(t, srv, "/api/v1/assist/explain", `{"steps":[{"kind":"hash","field":"value.a"}]}`)
	if status != http.StatusOK || body["summary"] != "Hash a (SHA-256)" || body["provider"] != "rules" {
		t.Fatalf("recipe explain: %d %v", status, body)
	}
	if status, _, _ = post(t, srv, "/api/v1/assist/explain", `{}`); status != http.StatusUnprocessableEntity {
		t.Fatalf("empty explain: %d", status)
	}
}

func TestExportEndpoint(t *testing.T) {
	srv := server(t, 30)
	status, body, _ := post(t, srv, "/api/v1/assist/export-js", `{"steps":[{"kind":"rename","from":"value.a","to":"value.b"}]}`)
	step, _ := body["step"].(map[string]any)
	if status != http.StatusOK || step["kind"] != "js" || !strings.Contains(step["code"].(string), "function transform") {
		t.Fatalf("export: %d %v", status, body)
	}
	status, body, _ = post(t, srv, "/api/v1/assist/export-js", `{"steps":[{"kind":"library","ref":"x","version":1}]}`)
	errs, _ := body["errors"].([]any)
	if status != http.StatusUnprocessableEntity || len(errs) != 1 || errs[0].(map[string]any)["path"] != "/steps/0/kind" {
		t.Fatalf("unsupported export: %d %v", status, body)
	}
}

func TestInferMappingEndpoint(t *testing.T) {
	srv := server(t, 30)
	status, body, _ := post(t, srv, "/api/v1/preview/infer-mapping",
		`{"input":{"key":"k","value":{"cust_id":"c-1","debug":true}},"desiredOutput":{"customerId":"c-1","source":"web"}}`)
	if status != http.StatusOK {
		t.Fatalf("status %d %v", status, body)
	}
	if steps := body["steps"].([]any); len(steps) != 3 {
		t.Fatalf("steps %v", steps)
	}
	if issues := body["issues"].([]any); len(issues) != 0 {
		t.Fatalf("issues %v", issues)
	}
	if unmapped := body["unmapped"].([]any); len(unmapped) != 1 || unmapped[0] != "value.debug" {
		t.Fatalf("unmapped %v", unmapped)
	}
	status, body, _ = post(t, srv, "/api/v1/preview/infer-mapping", `{"input":{"value":{"a":1}},"desiredOutput":[1]}`)
	if status != http.StatusUnprocessableEntity || body["path"] != "/desiredOutput" {
		t.Fatalf("bad desired: %d %v", status, body)
	}
}
