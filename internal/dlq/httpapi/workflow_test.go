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
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/dlqtest"
	"github.com/eventrouter/router/internal/dlq/jobs"
	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
)

const userHeader = "X-Test-User"

// principalStage makes the X-Test-User header the request principal (requester vs approver).
type principalStage struct{}

func (principalStage) Wrap(_ api.RouteSpec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := uuid.Parse(r.Header.Get(userHeader))
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{UserID: id})))
	})
}

type staticGate struct{ decision dlq.GateDecision }

func (g staticGate) Check(context.Context, dlq.RetryTarget) (dlq.GateDecision, error) {
	return g.decision, nil
}

// workflowServer serves the module with a memory job queue and the given 4-eyes decision.
func workflowServer(t *testing.T, decision dlq.GateDecision) (*httptest.Server, *memRecorder, *jobs.Runner) {
	t.Helper()
	opener := dlqtest.Cluster(t)
	dlqtest.Produce(t, opener, `{"amount":200}`, `{"amount":5}`, `oops`)
	m := dlqtest.ManagerWith(t, opener, func(d *dlq.AssemblyDeps) { d.Gate = staticGate{decision: decision} })
	repo, log := jobs.NewMemoryRepository(time.Now), slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := &jobs.Runner{Repo: repo, Pages: m, Notifier: jobs.LogNotifier{Log: log}, Log: log, Now: time.Now}
	rec := &memRecorder{}
	responder := api.NewResponder(problem.NewCatalog(), log)
	mounter := api.NewMounter(responder, nil, []api.RouteMiddleware{principalStage{}, auditStage{rec: rec}})
	r := chi.NewRouter()
	mounter.Install(r)
	svc := &jobs.Service{Repo: repo, Preparer: m, Gate: m, Audit: dlq.ScopeAuditTrail{}}
	mounter.Mount(r, NewModule(Deps{Manager: m, Jobs: svc, Authorizer: allowAll{}, Recorder: rec, Responder: responder}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, rec, runner
}

func callAs(t *testing.T, srv *httptest.Server, user, method, path, body string, into any) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	req.Header.Set(userHeader, user)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if into != nil {
		dec := json.NewDecoder(resp.Body)
		dec.UseNumber()
		_ = dec.Decode(into)
	}
	return resp
}

const (
	requester = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	approver  = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

func TestGroupsAndCauseFiltersOverHTTP(t *testing.T) {
	srv, _, _ := workflowServer(t, dlq.GateAllow)
	var groups dlq.GroupPage
	if resp := call(t, srv, http.MethodGet, base+"/groups?class=data", "", &groups); resp.StatusCode != 200 || len(groups.Items) != 1 || groups.Items[0].Count != 3 {
		t.Fatalf("groups %d %+v", resp.StatusCode, groups)
	}
	var global dlq.GroupPage
	call(t, srv, http.MethodGet, "/api/v1/dlq/groups", "", &global)
	if len(global.Items) != 1 || global.Items[0].RouteID != dlqtest.RouteID || global.ByClass[record.ClassData] != 3 {
		t.Fatalf("global groups %+v", global)
	}
	var page dlq.ListPage
	call(t, srv, http.MethodGet, base+"?fingerprint="+groups.Items[0].Fingerprint, "", &page)
	if len(page.Items) != 3 || page.Items[0].Cause != groups.Items[0].Fingerprint {
		t.Fatalf("fingerprint list %+v", page.Items)
	}
	var none dlq.ListPage
	call(t, srv, http.MethodGet, base+"?class=router", "", &none)
	if len(none.Items) != 0 {
		t.Fatalf("router class %+v", none.Items)
	}
	for _, path := range []string{base + "?class=ours", base + "?fingerprint=xyz", base + "/groups?class=nope"} {
		var p problem.Details
		if resp := call(t, srv, http.MethodGet, path, "", &p); resp.StatusCode != 400 || !strings.HasPrefix(p.Path, "/query/") {
			t.Fatalf("%s: %d %+v", path, resp.StatusCode, p)
		}
	}
}

func TestEditDryRunAndReplayOverHTTP(t *testing.T) {
	srv, rec, _ := workflowServer(t, dlq.GateAllow)
	var page dlq.ListPage
	call(t, srv, http.MethodGet, base, "", &page)
	oops := page.Items[0].ID
	edit := `{"edit":{"value":{"encoding":"json","data":{"amount":12345678901}}},"dryRun":true}`
	var dry ReplayDTO
	if resp := callAs(t, srv, requester, http.MethodPost, base+"/"+oops+"/replay", edit, &dry); resp.StatusCode != 200 || dry.Result == nil {
		t.Fatalf("dry %d %+v", resp.StatusCode, dry)
	}
	if dry.Result.Outcome != dlq.DryWouldSend || !strings.Contains(string(mustJSON(t, dry.Result.Outputs[0].Value.Data)), "12345678901") {
		t.Fatalf("dry output lost precision %+v", dry.Result)
	}
	var bad problem.Details
	if resp := callAs(t, srv, requester, http.MethodPost, base+"/"+oops+"/replay", `{"edit":{"value":{"encoding":"utf8","data":"{nope"}}}`, &bad); resp.StatusCode != 400 || bad.Path != "/edit/value" {
		t.Fatalf("invalid edit %d %+v", resp.StatusCode, bad)
	}
	var done ReplayDTO
	body := `{"edit":{"value":{"encoding":"json","data":{"amount":900}}},"note":"fixed by hand"}`
	if resp := callAs(t, srv, requester, http.MethodPost, base+"/"+oops+"/replay", body, &done); resp.StatusCode != 200 || done.Report == nil || done.Report.Counts[dlq.OutcomeSent] != 1 {
		t.Fatalf("replay %d %+v", resp.StatusCode, done)
	}
	if len(rec.actions()[dlq.ActionDLQEditedReplayed]) != 1 || len(rec.actions()[dlq.ActionDLQReplayPreview]) != 1 {
		t.Fatalf("audit %+v", rec.actions())
	}
	var report dlq.DryRunReport
	if resp := call(t, srv, http.MethodPost, base+"/dry-run", `{"match":{"errorClass":["data"]}}`, &report); resp.StatusCode != 200 || report.Sampled != 2 {
		t.Fatalf("dry-run selection %d %+v", resp.StatusCode, report)
	}
}

func TestFourEyesReplayOverHTTP(t *testing.T) {
	srv, rec, runner := workflowServer(t, dlq.GateRequireApproval)
	var page dlq.ListPage
	call(t, srv, http.MethodGet, base, "", &page)
	var p problem.Details
	if resp := callAs(t, srv, requester, http.MethodPost, base+"/retry", `{"ids":["`+page.Items[2].ID+`"]}`, &p); resp.StatusCode != 409 || p.Code != string(dlq.CodeApprovalRequired) {
		t.Fatalf("sync retry %d %+v", resp.StatusCode, p)
	}
	var pending ReplayDTO
	body := `{"edit":{"value":{"encoding":"json","data":{"amount":700}}},"note":"please"}`
	if resp := callAs(t, srv, requester, http.MethodPost, base+"/"+page.Items[0].ID+"/replay", body, &pending); resp.StatusCode != 202 || pending.Job == nil {
		t.Fatalf("pending %d %+v", resp.StatusCode, pending)
	}
	var bulk jobs.Job
	if resp := callAs(t, srv, requester, http.MethodPost, base+"/jobs", `{"action":"retry","ids":["`+page.Items[2].ID+`"],"ratePerSecond":5}`, &bulk); resp.StatusCode != 202 || bulk.Status != jobs.StatusPendingApproval || bulk.Rate != 5 {
		t.Fatalf("bulk %d %+v", resp.StatusCode, bulk)
	}
	var list JobList
	call(t, srv, http.MethodGet, "/api/v1/dlq/jobs?status=pending_approval", "", &list)
	if len(list.Items) != 2 {
		t.Fatalf("pending list %+v", list)
	}
	if resp := callAs(t, srv, requester, http.MethodPost, "/api/v1/dlq/jobs/"+pending.Job.ID+"/approve", `{}`, &p); resp.StatusCode != 403 || p.Code != string(dlq.CodeSelfApproval) {
		t.Fatalf("self approval %d %+v", resp.StatusCode, p)
	}
	var approved jobs.Job
	if resp := callAs(t, srv, approver, http.MethodPost, "/api/v1/dlq/jobs/"+pending.Job.ID+"/approve", `{"note":"ok"}`, &approved); resp.StatusCode != 200 || approved.Status != jobs.StatusQueued {
		t.Fatalf("approve %d %+v", resp.StatusCode, approved)
	}
	var rejected jobs.Job
	callAs(t, srv, approver, http.MethodPost, "/api/v1/dlq/jobs/"+bulk.ID+"/reject", `{"note":"no"}`, &rejected)
	if rejected.Status != jobs.StatusRejected {
		t.Fatalf("reject %+v", rejected)
	}
	if !runner.Tick(context.Background()) || runner.Tick(context.Background()) {
		t.Fatal("exactly the approved job must run")
	}
	var after dlq.ListPage
	call(t, srv, http.MethodGet, base, "", &after)
	if len(after.Items) != 2 {
		t.Fatalf("approved replay not applied: %d left", len(after.Items))
	}
	acts := rec.actions()
	if len(acts[dlq.ActionDLQReplayRequest]) != 2 || len(acts[dlq.ActionDLQReplayApproved]) != 1 || len(acts[dlq.ActionDLQReplayRejected]) != 1 {
		t.Fatalf("audit %+v", acts)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEditBodyKeepsNumberPrecision(t *testing.T) {
	edit, err := EditOf{}.Edit(&EditBody{Value: &EditValueBody{Encoding: dlq.EncodingJSON, Data: json.RawMessage(`{"id":12345678901234567890}`)},
		Key: &EditValueBody{Encoding: dlq.EncodingUTF8, Data: json.RawMessage(`"k-1"`)}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := dlq.EditBytes{}.Of(*edit.Value, dlq.PathEditValue)
	if string(b) != `{"id":12345678901234567890}` || edit.Key.Data != "k-1" {
		t.Fatalf("edit %s %+v", b, edit.Key)
	}
	if _, err := (EditOf{}).Edit(&EditBody{Key: &EditValueBody{Encoding: dlq.EncodingUTF8, Data: json.RawMessage(`5`)}}); err == nil {
		t.Fatal("text edit with a number accepted")
	}
}
