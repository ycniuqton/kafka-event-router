package sandboxinfo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eventrouter/router/pkg/sandbox"
)

func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func checkRouter(t *testing.T, checker SourceChecker) http.Handler {
	t.Helper()
	cfg := sandbox.DefaultConfig()
	mod, err := NewModule(Deps{Info: StaticInfo{Config: cfg}, Checker: checker,
		MaxSource: sandbox.NewLimitPolicy(cfg.Maxima).Defaults.MaxSourceBytes, Responder: testResponder()})
	if err != nil {
		t.Fatal(err)
	}
	return newRouter(t, nil, mod)
}

func TestCheckHandlerWithEngine(t *testing.T) {
	lazy := &LazyChecker{Config: sandbox.DefaultConfig()}
	t.Cleanup(func() { _ = lazy.Close(context.Background()) })
	h := checkRouter(t, lazy)
	cases := []struct {
		name, body string
		status     int
		diags      int
		line       int
	}{
		{"compiles", `{"code":"function transform(e, ctx) { return e; }","entry":"transform"}`, http.StatusOK, 0, 0},
		{"entry defaults to transform", `{"code":"function nope() {}"}`, http.StatusOK, 1, 0},
		{"syntax error with position", `{"code":"function transform(e) {\n  let x = ;\n}"}`, http.StatusOK, 1, 2},
		{"predicate", `{"code":"event.value.n > 1 &&","entry":"predicate"}`, http.StatusOK, 1, 1},
		{"unknown entry", `{"code":"x","entry":"main"}`, http.StatusUnprocessableEntity, 0, 0},
		{"too large", `{"code":"` + strings.Repeat("x", 65*1024) + `"}`, http.StatusRequestEntityTooLarge, 0, 0},
		{"body above limit", `{"code":"` + strings.Repeat("x", 400*1024) + `"}`, http.StatusRequestEntityTooLarge, 0, 0},
		{"malformed", `{"code":`, http.StatusBadRequest, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := post(h, string(PathCheck), c.body)
			if w.Code != c.status {
				t.Fatalf("status %d want %d: %s", w.Code, c.status, w.Body)
			}
			if c.status != http.StatusOK {
				assertProblem(t, w, c.name)
				return
			}
			var res CheckResponse
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.Diagnostics == nil {
				t.Fatalf("body %s (%v)", w.Body, err)
			}
			if len(res.Diagnostics) != c.diags || (c.line > 0 && res.Diagnostics[0].Line != c.line) {
				t.Fatalf("diagnostics %+v", res.Diagnostics)
			}
		})
	}
}

func assertProblem(t *testing.T, w *httptest.ResponseRecorder, name string) {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || p["type"] == nil || p["status"] == nil {
		t.Fatalf("%s: not a problem: %s", name, w.Body)
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatalf("%s: content type %q", name, w.Header().Get("Content-Type"))
	}
}

type brokenChecker struct{}

func (brokenChecker) Check(context.Context, sandbox.Source) ([]sandbox.Diagnostic, error) {
	return nil, errors.New("no engine")
}

func TestCheckHandlerUnavailableAndOptional(t *testing.T) {
	w := post(checkRouter(t, brokenChecker{}), string(PathCheck), `{"code":"1"}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("broken engine: %d %s", w.Code, w.Body)
	}
	mod, _ := NewModule(Deps{Info: StaticInfo{Config: sandbox.DefaultConfig()}, Responder: testResponder()})
	if w := post(newRouter(t, nil, mod), string(PathCheck), `{"code":"1"}`); w.Code == http.StatusOK {
		t.Fatal("without a checker the check route must not be mounted")
	}
}
