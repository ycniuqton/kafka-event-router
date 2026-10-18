package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/dlqtest"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
)

type allowAll struct{}

func (allowAll) Authorize(context.Context, auth.Principal, auth.Permission, auth.ObjectRef) error {
	return nil
}

type memRecorder struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (m *memRecorder) Record(_ context.Context, e audit.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, e)
	return nil
}

// auditStage opens an audit scope on mutating routes, like the platform's audit stage.
type auditStage struct{ rec *memRecorder }

func (s auditStage) Wrap(spec api.RouteSpec, next http.Handler) http.Handler {
	if !spec.Mutating {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(audit.WithScope(r.Context(), audit.NewScope(audit.RequestContext{}, s.rec, nil))))
	})
}

func server(t *testing.T) (*httptest.Server, *memRecorder) {
	t.Helper()
	opener := dlqtest.Cluster(t)
	dlqtest.Produce(t, opener, `{"amount":200}`, `{"amount":5}`, `oops`)
	rec := &memRecorder{}
	responder := api.NewResponder(problem.NewCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	mounter := api.NewMounter(responder, nil, []api.RouteMiddleware{auditStage{rec: rec}})
	r := chi.NewRouter()
	mounter.Install(r)
	mounter.Mount(r, NewModule(Deps{Manager: dlqtest.Manager(t, opener), Authorizer: allowAll{}, Recorder: rec, Responder: responder}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, rec
}

func call(t *testing.T, srv *httptest.Server, method, path, body string, into any) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if into != nil {
		_ = json.NewDecoder(resp.Body).Decode(into)
	}
	return resp
}

const base = "/api/v1/routes/" + dlqtest.RouteID + "/dlq"

func TestListDetailAndErrors(t *testing.T) {
	srv, _ := server(t)
	var page dlq.ListPage
	if resp := call(t, srv, http.MethodGet, base+"?limit=2", "", &page); resp.StatusCode != 200 || len(page.Items) != 2 || page.NextCursor == nil {
		t.Fatalf("list %d %+v", resp.StatusCode, page)
	}
	var rest dlq.ListPage
	call(t, srv, http.MethodGet, base+"?limit=2&cursor="+*page.NextCursor, "", &rest)
	if len(rest.Items) != 1 || rest.NextCursor != nil {
		t.Fatalf("page 2 %+v", rest)
	}
	var detail dlq.DetailView
	if resp := call(t, srv, http.MethodGet, base+"/"+page.Items[0].ID, "", &detail); resp.StatusCode != 200 || detail.ID != page.Items[0].ID {
		t.Fatalf("detail %d", resp.StatusCode)
	}
	cases := map[string]int{
		base + "/!!":                      http.StatusBadRequest,
		base + "?errorType=nope":          http.StatusBadRequest,
		base + "?since=yesterday":         http.StatusBadRequest,
		base + "?cursor=zzz":              http.StatusBadRequest,
		"/api/v1/routes/nope/dlq":         http.StatusNotFound,
		"/api/v1/routes/nope/dlq/summary": http.StatusNotFound,
	}
	for path, status := range cases {
		var p problem.Details
		if resp := call(t, srv, http.MethodGet, path, "", &p); resp.StatusCode != status {
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
	}
}

func TestRetryDiscardExportOverHTTP(t *testing.T) {
	srv, rec := server(t)
	var p problem.Details
	if resp := call(t, srv, http.MethodPost, base+"/retry", `{"ids":["x"],"match":{}}`, &p); resp.StatusCode != 400 || p.Path != "" {
		t.Fatalf("both: %d %+v", resp.StatusCode, p)
	}
	var retry RetryDTO
	if resp := call(t, srv, http.MethodPost, base+"/retry", `{"match":{"errorType":["step_error"]},"expectedCount":3}`, &retry); resp.StatusCode != 200 {
		t.Fatalf("retry %d", resp.StatusCode)
	}
	if retry.Counts[dlq.OutcomeSent] != 1 || retry.Counts[dlq.OutcomeFiltered] != 1 || retry.Counts[dlq.OutcomeFailed] != 1 || retry.ActiveVersionNo != 1 {
		t.Fatalf("retry %+v", retry)
	}
	var page dlq.ListPage
	call(t, srv, http.MethodGet, base, "", &page)
	if len(page.Items) != 1 {
		t.Fatalf("after retry %d", len(page.Items))
	}
	resp := call(t, srv, http.MethodGet, base+"/export?ids="+page.Items[0].ID, "", nil)
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Disposition"), "dlq-orders-") {
		t.Fatalf("export %d %v", resp.StatusCode, resp.Header)
	}
	var discard DiscardDTO
	call(t, srv, http.MethodPost, base+"/discard", `{"ids":["`+page.Items[0].ID+`"]}`, &discard)
	if discard.Discarded != 1 {
		t.Fatalf("discard %+v", discard)
	}
	var sum dlq.Summary
	call(t, srv, http.MethodGet, base+"/summary", "", &sum)
	var global dlq.GlobalSummary
	call(t, srv, http.MethodGet, "/api/v1/dlq/summary", "", &global)
	if sum.Unresolved != 0 || global.Unresolved != 0 {
		t.Fatalf("summary %+v %+v", sum, global)
	}
	actions := map[audit.Action]bool{}
	for _, e := range rec.entries {
		actions[e.Action] = true
	}
	if !actions[audit.ActionDLQRetried] || !actions[audit.ActionDLQDiscarded] || !actions[audit.ActionDLQExported] {
		t.Fatalf("audit %+v", rec.entries)
	}
}

func TestGlobalList(t *testing.T) {
	srv, _ := server(t)
	var page dlq.ListPage
	if resp := call(t, srv, http.MethodGet, "/api/v1/dlq?limit=2", "", &page); resp.StatusCode != 200 || len(page.Items) != 2 || page.NextCursor == nil {
		t.Fatalf("global %d %+v", resp.StatusCode, page)
	}
	var rest dlq.ListPage
	call(t, srv, http.MethodGet, "/api/v1/dlq?limit=2&cursor="+*page.NextCursor, "", &rest)
	if len(rest.Items) != 1 || rest.NextCursor != nil || rest.Items[0].ID == page.Items[0].ID {
		t.Fatalf("global page 2 %+v", rest)
	}
	var none dlq.ListPage
	call(t, srv, http.MethodGet, "/api/v1/dlq?routeId=other", "", &none)
	if len(none.Items) != 0 {
		t.Fatal("route filter ignored")
	}
}
