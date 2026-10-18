package httpapi

import (
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

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/dlqtest"
	"github.com/eventrouter/router/internal/dlq/jobs"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
)

// p2Server serves the module with masking (visibility) and a memory job queue whose runner the test drives.
func p2Server(t *testing.T, visibility dlq.SampleVisibility) (*httptest.Server, *memRecorder, *jobs.Runner) {
	t.Helper()
	opener := dlqtest.Cluster(t)
	dlqtest.Produce(t, opener, `{"amount":200,"email":"a@b.c"}`, `{"amount":5}`, `oops`)
	m := dlqtest.ManagerWith(t, opener, func(d *dlq.AssemblyDeps) { d.Visibility = visibility })
	repo := jobs.NewMemoryRepository(time.Now)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := &jobs.Runner{Repo: repo, Pages: m, Notifier: jobs.LogNotifier{Log: log}, Log: log, Now: time.Now}
	rec := &memRecorder{}
	responder := api.NewResponder(problem.NewCatalog(), log)
	mounter := api.NewMounter(responder, nil, []api.RouteMiddleware{auditStage{rec: rec}})
	r := chi.NewRouter()
	mounter.Install(r)
	mounter.Mount(r, NewModule(Deps{Manager: m, Jobs: &jobs.Service{Repo: repo, Preparer: m}, Authorizer: allowAll{}, Recorder: rec, Responder: responder}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, rec, runner
}

func (m *memRecorder) actions() map[audit.Action][]audit.Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[audit.Action][]audit.Entry{}
	for _, e := range m.entries {
		out[e.Action] = append(out[e.Action], e)
	}
	return out
}

// TestViewerSeesMaskedDetailAndExport is DL-125: masked detail + export keep types; views and exports are audited with masked=true.
func TestViewerSeesMaskedDetailAndExport(t *testing.T) {
	srv, rec, _ := p2Server(t, dlq.AlwaysMasked{})
	var page dlq.ListPage
	call(t, srv, http.MethodGet, base+"?q=m", "", &page)
	var detail dlq.DetailView
	call(t, srv, http.MethodGet, base+"/"+page.Items[2].ID, "", &detail)
	raw, _ := json.Marshal(detail.Value.Data)
	if !detail.Masked || string(raw) != `{"amount":0,"email":"*****"}` || detail.Key.Data != "**" {
		t.Fatalf("detail masked=%v value=%s key=%v", detail.Masked, raw, detail.Key.Data)
	}
	resp := call(t, srv, http.MethodGet, base+"/export?format=ndjson", "", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != string(dlq.MediaNDJSON) {
		t.Fatalf("export %d %v", resp.StatusCode, resp.Header)
	}
	got := rec.actions()
	viewed, exported := got[audit.ActionDLQViewed], got[audit.ActionDLQExported]
	if len(viewed) != 1 || !strings.Contains(string(viewed[0].Diff), `"masked":true`) || len(exported) != 1 || !strings.Contains(string(exported[0].Diff), `"masked":true`) {
		t.Fatalf("audit viewed=%+v exported=%+v", viewed, exported)
	}
}

func TestFiltersAndRestoreOverHTTP(t *testing.T) {
	srv, rec, _ := p2Server(t, nil)
	var page dlq.ListPage
	if call(t, srv, http.MethodGet, base+"?step=s&version=1&q=M", "", &page); len(page.Items) != 3 {
		t.Fatalf("filters %d", len(page.Items))
	}
	var p problem.Details
	if resp := call(t, srv, http.MethodGet, base+"?version=0", "", &p); resp.StatusCode != 400 || p.Path != string(dlq.PathVersion) {
		t.Fatalf("bad version %d %+v", resp.StatusCode, p)
	}
	id := page.Items[0].ID
	call(t, srv, http.MethodPost, base+"/discard", `{"ids":["`+id+`"]}`, nil)
	var resolved dlq.ListPage
	call(t, srv, http.MethodGet, "/api/v1/dlq?includeResolved=true", "", &resolved)
	if resolved.Items[0].Resolution == nil || resolved.Items[0].Resolution.State != dlq.StateDiscarded {
		t.Fatalf("global resolved %+v", resolved.Items[0])
	}
	var restore RestoreDTO
	call(t, srv, http.MethodPost, base+"/restore", `{"match":{}}`, &restore)
	if restore.Restored != 1 || len(restore.Skipped) != 2 {
		t.Fatalf("restore %+v", restore)
	}
	if call(t, srv, http.MethodGet, base, "", &page); len(page.Items) != 3 || len(rec.actions()[dlq.ActionDLQRestored]) != 1 {
		t.Fatalf("after restore %d", len(page.Items))
	}
}

func TestBulkJobOverHTTP(t *testing.T) {
	srv, _, runner := p2Server(t, nil)
	var p problem.Details
	if resp := call(t, srv, http.MethodPost, base+"/jobs", `{"action":"restore","match":{}}`, &p); resp.StatusCode != 400 {
		t.Fatalf("restore job %d", resp.StatusCode)
	}
	var job jobs.Job
	if resp := call(t, srv, http.MethodPost, base+"/jobs", `{"action":"discard","match":{"errorType":["step_error"]}}`, &job); resp.StatusCode != http.StatusAccepted || job.Status != jobs.StatusQueued {
		t.Fatalf("create %d %+v", resp.StatusCode, job)
	}
	if !runner.Tick(context.Background()) {
		t.Fatal("no job claimed")
	}
	call(t, srv, http.MethodGet, "/api/v1/dlq/jobs/"+job.ID, "", &job)
	if job.Status != jobs.StatusSucceeded || job.Processed != 3 || job.Total != 3 || job.Counts[dlq.OutcomeDiscarded] != 3 {
		t.Fatalf("job %+v", job)
	}
	var list JobList
	call(t, srv, http.MethodGet, base+"/jobs", "", &list)
	var page dlq.ListPage
	call(t, srv, http.MethodGet, base, "", &page)
	if len(list.Items) != 1 || len(page.Items) != 0 {
		t.Fatalf("list %d left %d", len(list.Items), len(page.Items))
	}
	var cancelled jobs.Job
	call(t, srv, http.MethodPost, base+"/jobs", `{"action":"retry","match":{}}`, &cancelled)
	call(t, srv, http.MethodPost, "/api/v1/dlq/jobs/"+cancelled.ID+"/cancel", "", &cancelled)
	if cancelled.Status != jobs.StatusCancelled || runner.Tick(context.Background()) {
		t.Fatalf("cancel %+v", cancelled)
	}
}
