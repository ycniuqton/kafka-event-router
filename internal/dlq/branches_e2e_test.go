package dlq

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

const branchRouteConfig = `{
  "apiVersion": "router/v1", "name": "orders",
  "source": {"connectionId": "c1", "topics": ["orders"], "format": {"type": "json"}},
  "branches": [
    {"id": "main", "destination": {"connectionId": "c1", "topic": "orders-main", "format": {"type": "json"}}},
    {"id": "audit", "destination": {"connectionId": "c1", "topic": "orders-audit", "format": {"type": "json"}}}],
  "errorPolicy": {"onError": "dlq", "dlqTopic": "router.dlq.orders"}
}`

// produceBranchFailures writes two dead letters of branch main: one where the worker had already delivered
// branch audit (router.delivered_branches=audit) and one without the header (nothing delivered).
func produceBranchFailures(t *testing.T, opener SeedClientOpener) {
	t.Helper()
	cl, err := opener.Open(context.Background(), "c1", "")
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	for i, delivered := range []record.BranchSet{record.BranchSetOf("audit"), ""} {
		rec := failure(i, record.ErrorOutputInvalid)
		rec.Failure.Delivered = delivered
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

func countTopic(t *testing.T, opener SeedClientOpener, topic string) int {
	t.Helper()
	cl, err := opener.Open(context.Background(), "c1", "", kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got := 0
	for ctx.Err() == nil {
		cl.PollFetches(ctx).EachRecord(func(*kgo.Record) { got++ })
	}
	return got
}

// TestBranchAwareRetryReemitsOnlyFailedBranch is DL-126: healthy branches already delivered are not re-emitted.
func TestBranchAwareRetryReemitsOnlyFailedBranch(t *testing.T) {
	_, opener := newCluster(t, testDLQTopic, "orders-main", "orders-audit")
	produceBranchFailures(t, opener)
	routes := fakeRoutes{route: RouteInfo{ID: "r1", Name: "orders", TeamID: "t1", ActiveVersionID: "v1"},
		versions: []VersionConfig{{ID: "v1", No: 1, Config: json.RawMessage(branchRouteConfig)}}}
	m, err := NewAssembly(AssemblyDeps{
		Settings: Settings{StateBackend: StateBackendMemory, Executor: ExecutorInProcess, Scripts: ScriptsNone},
		Routes:   routes, Clients: opener, Compiler: compile.NewDefaultCompiler(), Scripts: NoScriptRuntimes{},
		Salts: ctxhelpers.StaticSaltProvider{}, Audit: NoAuditTrail{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report, err := m.Act(context.Background(), ActionRetry, Actor{UserID: "u1"}, "r1", Selection{Match: &Query{}})
	if err != nil || report.Counts()[OutcomeSent] != 2 {
		t.Fatalf("retry %v %+v", err, report)
	}
	if main, audit := countTopic(t, opener, "orders-main"), countTopic(t, opener, "orders-audit"); main != 2 || audit != 1 {
		t.Fatalf("main=%d audit=%d (want 2 and 1: the delivered audit branch is not re-emitted)", main, audit)
	}
}
