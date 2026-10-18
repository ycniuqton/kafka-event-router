package routeshttp_test

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
)

// checkReplay (routes#p3-be-replay): missing confirm → 422 at /confirm, 202 pending replay audited,
// a second open replay → 409, status by id, unknown replay → 404.
func (h *harness) checkReplay(item string) {
	t := h.t
	from := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	bad := h.expect(h.do(http.MethodPost, item+"/replay", map[string]any{"from": from}, ""), http.StatusUnprocessableEntity)
	if errs := asList(bad.body["errors"]); len(errs) != 1 || errs[0].(map[string]any)["path"] != "/confirm" {
		t.Fatal(bad.raw)
	}
	ok := h.expect(h.do(http.MethodPost, item+"/replay", map[string]any{"from": from, "confirm": true}, ""), http.StatusAccepted)
	id, _ := ok.body["replayId"].(string)
	if id == "" || ok.body["status"] != "pending" {
		t.Fatal(ok.raw)
	}
	h.expect(h.do(http.MethodPost, item+"/replay", map[string]any{"from": from, "confirm": true}, ""), http.StatusConflict)
	got := h.expect(h.do(http.MethodGet, item+"/replays/"+id, nil, ""), http.StatusOK)
	if got.body["replayId"] != id {
		t.Fatal(got.raw)
	}
	h.expect(h.do(http.MethodGet, item+"/replays/"+uuid.NewString(), nil, ""), http.StatusNotFound)
	if !h.audited(audit.ActionRouteReplayed) {
		t.Fatal("replay was not audited")
	}
}

// audited reports whether an audit entry with action a was recorded.
func (h *harness) audited(a audit.Action) bool {
	h.audit.mu.Lock()
	defer h.audit.mu.Unlock()
	for _, e := range h.audit.entries {
		if e.Action == a {
			return true
		}
	}
	return false
}
