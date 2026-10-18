package dlq

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

const e2eRouteConfig = `{
  "apiVersion": "router/v1", "name": "orders",
  "source": {"connectionId": "c1", "topics": ["orders"], "format": {"type": "json"}},
  "branches": [{"id": "main",
    "filter": {"type": "cond", "field": "value.amount", "op": "gt", "value": 100},
    "transform": {"steps": [{"kind": "rename", "from": "value.cust_id", "to": "value.customerId"}]},
    "destination": {"connectionId": "c1", "topic": "orders-public", "format": {"type": "json"}}}],
  "errorPolicy": {"onError": "dlq", "dlqTopic": "router.dlq.orders"}
}`

type fakeRoutes struct {
	route    RouteInfo
	versions []VersionConfig
}

func (f fakeRoutes) Route(_ context.Context, id string) (RouteInfo, error) {
	if id != f.route.ID {
		return RouteInfo{}, ErrRouteNotFound
	}
	return f.route, nil
}
func (f fakeRoutes) Routes(context.Context) ([]RouteInfo, error) { return []RouteInfo{f.route}, nil }
func (f fakeRoutes) Versions(context.Context, string) ([]VersionConfig, error) {
	return f.versions, nil
}
func (f fakeRoutes) Version(_ context.Context, id string) (VersionConfig, error) {
	for _, v := range f.versions {
		if v.ID == id {
			return v, nil
		}
	}
	return VersionConfig{}, errors.New("no version")
}

func e2eManager(t *testing.T, opener SeedClientOpener) *Manager {
	t.Helper()
	return e2eManagerWith(t, opener, e2eRouteConfig)
}

func e2eManagerWith(t *testing.T, opener SeedClientOpener, cfg string) *Manager {
	t.Helper()
	routes := fakeRoutes{
		route:    RouteInfo{ID: "r1", Name: "orders", TeamID: "t1", ActiveVersionID: "v1"},
		versions: []VersionConfig{{ID: "v1", No: 1, Config: json.RawMessage(cfg)}},
	}
	m, err := NewAssembly(AssemblyDeps{
		Settings: Settings{StateBackend: StateBackendMemory, Executor: ExecutorInProcess, Scripts: ScriptsNone},
		Routes:   routes, Clients: opener, Compiler: compile.NewDefaultCompiler(), Scripts: NoScriptRuntimes{},
		Salts: ctxhelpers.StaticSaltProvider{}, Audit: NoAuditTrail{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func produceValues(t *testing.T, opener SeedClientOpener, values []string) {
	t.Helper()
	cl, err := opener.Open(context.Background(), "c1", "")
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	for i, v := range values {
		rec := failure(i, record.ErrorStep)
		rec.Value = []byte(v)
		raw := record.Codec{}.Encode(rec)
		hs := []kgo.RecordHeader{}
		for _, h := range raw.Headers {
			hs = append(hs, kgo.RecordHeader{Key: h.Key, Value: h.Value})
		}
		if err := cl.ProduceSync(context.Background(), &kgo.Record{Topic: testDLQTopic, Key: raw.Key, Value: raw.Value, Headers: hs}).FirstErr(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestManagerEndToEnd(t *testing.T) {
	_, opener := newCluster(t, testDLQTopic, "orders-public")
	produceValues(t, opener, []string{`{"amount":200,"cust_id":1}`, `{"amount":50}`, `not json`, `{"amount":300,"cust_id":2}`, `{"amount":10}`, `{bad`})
	m := e2eManager(t, opener)
	ctx := context.Background()
	page, err := m.List(ctx, "r1", Query{})
	if err != nil || len(page.Items) != 6 || page.NextCursor != nil {
		t.Fatalf("list %v %+v", err, page)
	}
	detail, err := m.Get(ctx, Viewer{}, "r1", page.Items[2].ID)
	if err != nil || detail.Value.Encoding != EncodingJSON || detail.Key.Encoding != EncodingUTF8 || page.Items[0].Source.Offset != 5 {
		t.Fatalf("detail %v %+v", err, detail)
	}
	ids := []string{}
	for _, it := range page.Items {
		ids = append(ids, it.ID)
	}
	report, err := m.Act(ctx, ActionRetry, Actor{UserID: "u1"}, "r1", Selection{IDs: ids})
	counts := report.Counts()
	if err != nil || counts[OutcomeSent] != 2 || counts[OutcomeFiltered] != 2 || counts[OutcomeFailed] != 2 || report.ActiveVersionNo != 1 {
		t.Fatalf("retry %v %+v", err, report)
	}
	assertDestination(t, opener, 2)
	sentID := ""
	for _, res := range report.Results {
		if res.Outcome == OutcomeSent {
			sentID = RefCodec{}.Encode(res.Ref)
		}
	}
	again, _ := m.Act(ctx, ActionRetry, Actor{UserID: "u1"}, "r1", Selection{IDs: []string{sentID}})
	if again.Results[0].Reason != SkipAlreadyResolved {
		t.Fatalf("second retry %+v", again.Results)
	}
	left, _ := m.List(ctx, "r1", Query{})
	if len(left.Items) != 2 {
		t.Fatalf("left %d", len(left.Items))
	}
	exp, err := m.Export(ctx, Viewer{}, "r1", Selection{Match: &Query{}}, ExportJSON)
	if err != nil || len(exp.Views) != 2 || !strings.HasPrefix(exp.Name, "dlq-orders-") {
		t.Fatalf("export %v %d %s", err, len(exp.Views), exp.Name)
	}
	two := 2
	disc, err := m.Act(ctx, ActionDiscard, Actor{UserID: "u1"}, "r1", Selection{Match: &Query{}, ExpectedCount: &two})
	if err != nil || disc.Counts()[OutcomeDiscarded] != 2 {
		t.Fatalf("discard %v %+v", err, disc)
	}
	s, err := m.Summary(ctx, "r1")
	if err != nil || s.Unresolved != 0 || s.Approximate {
		t.Fatalf("summary %v %+v", err, s)
	}
	global, err := m.ListGlobal(ctx, []RouteInfo{{ID: "r1"}}, Query{IncludeResolved: true})
	if err != nil || len(global.Items) != 6 {
		t.Fatalf("global %v %d", err, len(global.Items))
	}
}

func TestManagerErrors(t *testing.T) {
	_, opener := newCluster(t, testDLQTopic, "orders-public")
	m := e2eManager(t, opener)
	ctx := context.Background()
	if _, err := m.List(ctx, "nope", Query{}); !errors.Is(err, ErrRouteNotFound) {
		t.Fatal(err)
	}
	if _, err := m.List(ctx, "r1", Query{Cursor: "@@"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatal(err)
	}
	if _, err := m.Get(ctx, Viewer{}, "r1", RefCodec{}.Encode(Ref{Topic: "other", Offset: 1})); !errors.Is(err, ErrRecordNotFound) {
		t.Fatal(err)
	}
	one := 1
	if _, err := m.Act(ctx, ActionDiscard, Actor{}, "r1", Selection{Match: &Query{}, ExpectedCount: &one}); !errors.Is(err, ErrCountMismatch) {
		t.Fatal(err)
	}
	if _, err := m.Act(ctx, ActionDiscard, Actor{}, "r1", Selection{}); !errors.Is(err, ErrSelectionShape) {
		t.Fatal(err)
	}
	if _, err := m.Export(ctx, Viewer{}, "r1", Selection{IDs: []string{"x"}}, "xml"); !errors.Is(err, ErrUnknownFormat) {
		t.Fatal(err)
	}
}

func assertDestination(t *testing.T, opener SeedClientOpener, want int) {
	t.Helper()
	cl, err := opener.Open(context.Background(), "c1", "", kgo.ConsumeTopics("orders-public"), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	got := 0
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for got < want && ctx.Err() == nil {
		fs := cl.PollFetches(ctx)
		fs.EachRecord(func(r *kgo.Record) {
			if !strings.Contains(string(r.Value), "customerId") {
				t.Errorf("not transformed: %s", r.Value)
			}
			got++
		})
	}
	if got != want {
		t.Fatalf("destination got %d", got)
	}
}
