package password

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
)

func gateServe(t *testing.T, spec api.RouteSpec, p auth.Principal) *httptest.ResponseRecorder {
	t.Helper()
	gate := GateStage{Allowed: ChangeAllowed, Responder: api.NewResponder(problem.NewCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))}
	h := gate.Wrap(spec, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	r := httptest.NewRequest(spec.Method, "/x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	return w
}

func TestGateBlocksEverythingButTheAllowList(t *testing.T) {
	pending := auth.Principal{Kind: auth.PrincipalUser, MustChangePassword: true}
	w := gateServe(t, api.RouteSpec{Method: "GET", Pattern: "/api/v1/routes"}, pending)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != http.StatusForbidden || body["code"] != string(CodeChangeRequired) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	for _, spec := range []api.RouteSpec{{Method: "GET", Pattern: "/api/v1/auth/me"}, {Method: "POST", Pattern: "/api/v1/auth/password"},
		{Method: "POST", Pattern: "/auth/logout", Auth: api.AuthOptional}, {Method: "POST", Pattern: "/auth/password/login", Auth: api.AuthNone}} {
		if w := gateServe(t, spec, pending); w.Code != http.StatusTeapot {
			t.Errorf("%s %s blocked: %d", spec.Method, spec.Pattern, w.Code)
		}
	}
}

func TestGatePassesUsersWithoutPendingChange(t *testing.T) {
	w := gateServe(t, api.RouteSpec{Method: "GET", Pattern: "/api/v1/routes"}, auth.Principal{Kind: auth.PrincipalUser})
	if w.Code != http.StatusTeapot {
		t.Fatalf("status=%d", w.Code)
	}
}
