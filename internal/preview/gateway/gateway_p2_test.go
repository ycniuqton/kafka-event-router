package gateway_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/platform/ratelimit"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/preview/gateway"
	"github.com/eventrouter/router/internal/rbac"
)

// sensitiveSamples masks connection "sensitive" (fails for "broken", does not know "unknown"), like
// access-control's SampleVisibility for an editor.
type sensitiveSamples struct{}

func (sensitiveSamples) Decide(_ context.Context, _ rbac.SampleSurface, conn string) (rbac.SampleDecision, error) {
	switch conn {
	case "broken":
		return rbac.SampleDecision{Masked: true}, errors.New("directory down")
	case "unknown":
		return rbac.SampleDecision{Masked: true}, rbac.ErrUnknownConnection
	}
	return rbac.SampleDecision{Masked: conn == "sensitive"}, nil
}

// topicAuthz allows everything except topic.create on connection "locked".
type topicAuthz struct{}

func (topicAuthz) Authorize(_ context.Context, _ auth.Principal, perm auth.Permission, obj auth.ObjectRef) error {
	if perm.Key == string(rbac.PermTopicCreate) && obj.ID == "locked" {
		return auth.ErrForbidden
	}
	return nil
}

type p2Gateway struct {
	*fixture
	direct *memRecorder
}

func newP2Gateway(t *testing.T, rule ratelimit.Rule) *p2Gateway {
	t.Helper()
	f := &fixture{recorder: &memRecorder{}, status: http.StatusOK, reply: `{"summary":{},"testTopic":{"connectionId":"dest","topic":"router.test.x.1","produced":2}}`,
		sessions: map[string]string{"ps_1": `{"id":"ps_1","connectionId":"s","mode":"latest"}`}}
	g := &p2Gateway{fixture: f, direct: &memRecorder{}}
	codec := preview.NewDirectiveCodec("s", time.Now)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, _ := codec.Decode(r.Header.Get(string(preview.HeaderCaller)))
		body, _ := io.ReadAll(r.Body)
		f.calls = append(f.calls, upstreamCall{path: r.URL.RequestURI(), caller: caller, body: body})
		w.Header().Set("Content-Type", "application/json")
		if f.sessionSource(w, r) {
			return
		}
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.reply))
	}))
	t.Cleanup(up.Close)
	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := api.NewResponder(problem.NewCatalog(), log)
	mw := audit.NewMiddleware(f.recorder, audit.NewDiffBuilder(logging.NewRedactor()), api.AuditContext, log)
	user := auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New(), Teams: []auth.TeamRole{{TeamID: uuid.New(), Role: auth.RoleEditor}}}
	m := api.NewMounter(responder, nil, []api.RouteMiddleware{principalStage{p: user}, api.AuditStage{Audit: mw}})
	f.router = chi.NewRouter()
	m.Install(f.router)
	m.Mount(f.router, gateway.NewModule(gateway.Deps{
		Client: gateway.NewPreviewClient(up.URL, nil, codec), Responder: responder,
		Authorizer: gateway.PreviewAuthorizer{Authz: topicAuthz{}, Samples: sensitiveSamples{}},
		Limiter:    gateway.NewPreviewRateLimiter(ratelimit.NewRedisLimiter(rdb), rule, log),
		TestTopics: gateway.TestTopicAuditor{Recorder: g.direct},
	}))
	return g
}

var roomyRule = ratelimit.Rule{PerMinute: 6000, Burst: 1000}

func runBody(source, dest string, write bool) map[string]any {
	return map[string]any{"sessionId": "ps_1", "writeToTopic": write, "config": map[string]any{
		"name": "r", "source": map[string]any{"connectionId": source},
		"branches": []any{map[string]any{"id": "main", "destination": map[string]any{"connectionId": dest}}},
	}}
}

func TestDirectiveMaskFollowsSampleVisibility(t *testing.T) {
	g := newP2Gateway(t, roomyRule)
	g.status, g.reply = http.StatusCreated, `{"session":{"id":"ps_1","loaded":1,"masked":true},"events":[]}`
	cases := []struct {
		conn string
		want bool
	}{{"plain", false}, {"sensitive", true}, {"broken", true}}
	for _, c := range cases {
		g.do(http.MethodPost, "/api/v1/preview/samples", map[string]any{"mode": "latest", "connectionId": c.conn, "topics": []string{"t"}})
		if got := g.calls[len(g.calls)-1].caller.Mask; got != c.want {
			t.Fatalf("load on %s: mask = %v", c.conn, got)
		}
	}
	if e := g.recorder.entries[len(g.recorder.entries)-1]; e.Action != audit.ActionSampleViewed {
		t.Fatalf("masked load must audit sample.viewed: %+v", e)
	}
	g.status, g.reply = http.StatusOK, `{"summary":{}}`
	g.do(http.MethodPost, "/api/v1/preview/run", inlineBody("sensitive", ""))
	if !g.calls[len(g.calls)-1].caller.Mask {
		t.Fatal("inline run on a sensitive draft source must be masked")
	}
}

func inlineBody(source string, sampleSession string) map[string]any {
	body := map[string]any{"events": []any{map[string]any{"value": map[string]any{"a": 1}}}, "config": map[string]any{
		"name": "r", "source": map[string]any{"connectionId": source},
	}}
	if sampleSession != "" {
		body["sampleSessionId"] = sampleSession
	}
	return body
}

// TestRunMaskFollowsTheSessionSource: session runs and inline runs copied from a session are masked by the
// session's connection, not the draft's; upload sessions (no connection) are not masked for editors.
func TestRunMaskFollowsTheSessionSource(t *testing.T) {
	g := newP2Gateway(t, roomyRule)
	g.status, g.reply = http.StatusOK, `{"summary":{}}`
	g.sessions = map[string]string{
		"ps_up":  `{"id":"ps_up","mode":"upload"}`,
		"ps_sen": `{"id":"ps_sen","connectionId":"sensitive","mode":"latest"}`,
	}
	cases := []struct {
		name string
		body map[string]any
		want bool
	}{
		{"session run on an upload, sensitive draft", runSession("ps_up", "sensitive"), false},
		{"session run on a sensitive session, plain draft", runSession("ps_sen", "plain"), true},
		{"studio inline run copied from an upload", inlineBody("plain", "ps_up"), false},
		{"studio inline run copied from a sensitive session", inlineBody("plain", "ps_sen"), true},
		{"inline run without a session, plain draft", inlineBody("plain", ""), false},
	}
	for _, c := range cases {
		rec, body := g.do(http.MethodPost, "/api/v1/preview/run", c.body)
		last := g.calls[len(g.calls)-1]
		if rec.Code != http.StatusOK || last.path != "/internal/v1/preview/run" || last.caller.Mask != c.want {
			t.Fatalf("%s: %d %v mask=%v", c.name, rec.Code, body, last.caller.Mask)
		}
	}
}

func runSession(id, source string) map[string]any {
	return map[string]any{"sessionId": id, "config": map[string]any{"name": "r", "source": map[string]any{"connectionId": source}}}
}

func TestUnknownConnectionIsNotFoundNotMasked(t *testing.T) {
	g := newP2Gateway(t, roomyRule)
	g.sessions = map[string]string{"ps_gone": `{"id":"ps_gone","connectionId":"unknown","mode":"latest"}`}
	requests := []struct {
		path string
		body map[string]any
	}{
		{"/api/v1/preview/samples", map[string]any{"mode": "upload", "connectionId": "unknown", "upload": "[1]"}},
		{"/api/v1/preview/run", inlineBody("unknown", "")},
		{"/api/v1/preview/run", runSession("ps_gone", "plain")},
	}
	for _, req := range requests {
		before := len(g.calls)
		rec, body := g.do(http.MethodPost, req.path, req.body)
		if rec.Code != http.StatusNotFound || body["path"] != string(preview.FieldConnectionID) {
			t.Fatalf("%s %v: %d %v", req.path, req.body, rec.Code, body)
		}
		for _, call := range g.calls[before:] {
			if !strings.HasPrefix(call.path, "/internal/v1/preview/sessions/") {
				t.Fatalf("unknown connection reached the preview service: %s", call.path)
			}
		}
	}
	rec, _ := g.do(http.MethodPost, "/api/v1/preview/run", runSession("ps_missing", "plain"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing session: %d", rec.Code)
	}
}

func TestTestTopicNeedsTopicCreateAndIsAudited(t *testing.T) {
	g := newP2Gateway(t, roomyRule)
	rec, body := g.do(http.MethodPost, "/api/v1/preview/test-runs", runBody("s", "locked", true))
	if rec.Code != http.StatusForbidden || body["path"] != gateway.TestTopicField || len(g.calls) != 0 {
		t.Fatalf("locked destination: %d %v", rec.Code, body)
	}
	if rec, _ := g.do(http.MethodPost, "/api/v1/preview/test-runs", runBody("s", "locked", false)); rec.Code != http.StatusOK || len(g.direct.entries) != 0 {
		t.Fatalf("test run without topic: %d audits %d", rec.Code, len(g.direct.entries))
	}
	if rec, _ := g.do(http.MethodPost, "/api/v1/preview/test-runs", runBody("s", "dest", true)); rec.Code != http.StatusOK {
		t.Fatalf("allowed test topic: %d", rec.Code)
	}
	if len(g.direct.entries) != 1 {
		t.Fatalf("audit entries = %+v", g.direct.entries)
	}
	e := g.direct.entries[0]
	if e.Action != gateway.ActionTestTopicWritten || e.ObjectID != "router.test.x.1" || e.Object != audit.ObjectTopic {
		t.Fatalf("audit entry = %+v", e)
	}
}

func TestPreviewRateLimitPerUser(t *testing.T) {
	g := newP2Gateway(t, ratelimit.Rule{PerMinute: 60, Burst: 2})
	codes := []int{}
	for range 4 {
		rec, _ := g.do(http.MethodPost, "/api/v1/preview/run", runBody("s", "d", false))
		codes = append(codes, rec.Code)
	}
	if codes[0] != http.StatusOK || codes[3] != http.StatusTooManyRequests {
		t.Fatalf("codes = %v", codes)
	}
	if rec, _ := g.do(http.MethodGet, "/api/v1/preview/sessions/ps_1/events", nil); rec.Code != http.StatusOK {
		t.Fatalf("paging is not rate limited: %d", rec.Code)
	}
}
