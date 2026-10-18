package gateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/preview/gateway"
)

// viewerAuthz denies write permissions and allows reads (a viewer).
type viewerAuthz struct{ denyConnection bool }

func (a viewerAuthz) Authorize(_ context.Context, _ auth.Principal, perm auth.Permission, obj auth.ObjectRef) error {
	if perm.Access == auth.AccessWrite || (a.denyConnection && obj.Type == gateway.ObjectConnection) {
		return auth.ErrForbidden
	}
	return nil
}

type memRecorder struct{ entries []audit.Entry }

func (m *memRecorder) Record(_ context.Context, e audit.Entry) error {
	m.entries = append(m.entries, e)
	return nil
}

type upstreamCall struct {
	path   string
	caller preview.Caller
	body   []byte
}

type fixture struct {
	router   chi.Router
	recorder *memRecorder
	calls    []upstreamCall
	status   int
	reply    string
	sessions map[string]string
}

// sessionSource answers GET /internal/v1/preview/sessions/{id} from f.sessions (unknown ids → 404).
func (f *fixture) sessionSource(w http.ResponseWriter, r *http.Request) bool {
	id, ok := strings.CutPrefix(r.URL.Path, "/internal/v1/preview/sessions/")
	if r.Method != http.MethodGet || !ok || strings.Contains(id, "/") {
		return false
	}
	reply, found := f.sessions[id]
	if !found {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":404,"detail":"no session"}`))
		return true
	}
	_, _ = w.Write([]byte(reply))
	return true
}

func newFixture(t *testing.T, authz auth.Authorizer) *fixture {
	t.Helper()
	f := &fixture{recorder: &memRecorder{}, status: http.StatusCreated, reply: `{"session":{"id":"ps_1","loaded":3,"masked":false},"events":[]}`,
		sessions: map[string]string{"ps_1": `{"id":"ps_1","mode":"upload"}`}}
	codec := preview.NewDirectiveCodec("s", time.Now)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, err := codec.Decode(r.Header.Get(string(preview.HeaderCaller)))
		if err != nil {
			t.Errorf("upstream got bad directive: %v", err)
		}
		body, _ := io.ReadAll(r.Body)
		f.calls = append(f.calls, upstreamCall{path: r.URL.RequestURI(), caller: caller, body: body})
		w.Header().Set("Content-Type", "application/problem+json")
		if f.sessionSource(w, r) {
			return
		}
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.reply))
	}))
	t.Cleanup(up.Close)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := api.NewResponder(problem.NewCatalog(), log)
	mw := audit.NewMiddleware(f.recorder, audit.NewDiffBuilder(logging.NewRedactor()), api.AuditContext, log)
	user := auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New(), Teams: []auth.TeamRole{{TeamID: uuid.New(), Role: auth.RoleViewer}}}
	m := api.NewMounter(responder, nil, []api.RouteMiddleware{principalStage{p: user}, api.AuthorizeStage{Authorize: auth.NewAuthorizeMiddleware(authz), Responder: responder}, api.AuditStage{Audit: mw}})
	f.router = chi.NewRouter()
	m.Install(f.router)
	m.Mount(f.router, gateway.NewModule(gateway.Deps{
		Client: gateway.NewPreviewClient(up.URL, nil, codec), Authorizer: gateway.PreviewAuthorizer{Authz: authz}, Responder: responder,
	}))
	return f
}

type principalStage struct{ p auth.Principal }

func (s principalStage) Wrap(_ api.RouteSpec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), s.p)))
	})
}

func (f *fixture) do(method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, httptest.NewRequest(method, path, rd))
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestLoadForwardsAndAudits(t *testing.T) {
	f := newFixture(t, viewerAuthz{})
	rec, body := f.do(http.MethodPost, "/api/v1/preview/samples", map[string]any{"mode": "latest", "connectionId": "c1", "topics": []string{"orders"}, "count": 20})
	if rec.Code != http.StatusCreated || body["session"] == nil {
		t.Fatalf("status %d body %v", rec.Code, body)
	}
	if len(f.calls) != 1 || f.calls[0].path != "/internal/v1/preview/samples" || f.calls[0].caller.UserID == "" {
		t.Fatalf("upstream calls = %+v", f.calls)
	}
	if len(f.recorder.entries) != 1 {
		t.Fatalf("audit entries = %+v", f.recorder.entries)
	}
	e := f.recorder.entries[0]
	if e.Action != audit.ActionSampleViewedRaw || e.Object != audit.ObjectSampleSession || e.ObjectID != "ps_1" || e.TeamID == nil {
		t.Fatalf("audit entry = %+v", e)
	}
}

func TestLoadAuthorization(t *testing.T) {
	cases := []struct {
		name  string
		authz auth.Authorizer
		body  map[string]any
		path  string
	}{
		{name: "connection not usable", authz: viewerAuthz{denyConnection: true}, body: map[string]any{"mode": "latest", "connectionId": "c1", "topics": []string{"t"}}, path: "/connectionId"},
		{name: "upload needs editor", authz: viewerAuthz{}, body: map[string]any{"mode": "upload", "upload": "[1]"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.authz)
			rec, body := f.do(http.MethodPost, "/api/v1/preview/samples", tc.body)
			if rec.Code != http.StatusForbidden || len(f.calls) != 0 || len(f.recorder.entries) != 0 {
				t.Fatalf("status %d body %v calls %d", rec.Code, body, len(f.calls))
			}
			if tc.path != "" && body["path"] != tc.path {
				t.Fatalf("path = %v", body["path"])
			}
		})
	}
}

func TestForwardRelaysUpstreamProblems(t *testing.T) {
	f := newFixture(t, viewerAuthz{})
	f.status, f.reply = http.StatusGone, `{"type":"`+string(preview.TypeSessionExpired)+`","status":410}`
	rec, body := f.do(http.MethodGet, "/api/v1/preview/sessions/ps_9/events?limit=5&q=x", nil)
	if rec.Code != http.StatusGone || body["type"] != string(preview.TypeSessionExpired) {
		t.Fatalf("status %d body %v", rec.Code, body)
	}
	if f.calls[0].path != "/internal/v1/preview/sessions/ps_9/events?limit=5&q=x" {
		t.Fatalf("forwarded path = %s", f.calls[0].path)
	}
}

func TestRunForwardsBodyVerbatim(t *testing.T) {
	f := newFixture(t, viewerAuthz{})
	f.status, f.reply = http.StatusOK, `{"summary":{}}`
	payload := map[string]any{"sessionId": "ps_1", "config": map[string]any{"name": "x"}}
	rec, _ := f.do(http.MethodPost, "/api/v1/preview/run", payload)
	want, _ := json.Marshal(payload)
	if rec.Code != http.StatusOK || len(f.calls) != 2 || !bytes.Equal(bytes.TrimSpace(f.calls[1].body), want) {
		t.Fatalf("status %d calls %+v", rec.Code, f.calls)
	}
	if rec, _ := f.do(http.MethodPost, "/api/v1/preview/test-runs", payload); rec.Code != http.StatusForbidden {
		t.Fatalf("test run by viewer = %d", rec.Code)
	}
}

func TestUpstreamDownIs503(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := api.NewResponder(problem.NewCatalog(), log)
	m := api.NewMounter(responder, nil, []api.RouteMiddleware{principalStage{p: auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New()}}})
	r := chi.NewRouter()
	m.Mount(r, gateway.NewModule(gateway.Deps{
		Client:     gateway.NewPreviewClient("http://127.0.0.1:1", nil, preview.NewDirectiveCodec("s", time.Now)),
		Authorizer: gateway.PreviewAuthorizer{Authz: viewerAuthz{}}, Responder: responder,
	}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/preview/sessions/ps_1/schema", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
}
