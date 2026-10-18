package dlq

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/kafka"
)

const demoRouteConfig = `{
  "apiVersion": "router/v1", "name": "order-events",
  "source": {"connectionId": "c1", "topics": ["order-events"], "format": {"type": "json"}},
  "branches": [{"id": "main",
    "transform": {"steps": [{"kind": "rename", "from": "value.cust_id", "to": "value.customerId"}]},
    "destination": {"connectionId": "c1", "topic": "orders-clean", "format": {"type": "json"}}}]
}`

type demoSpecs struct{ kind kafka.ClusterKind }

func (s demoSpecs) Spec(_ context.Context, id string) (kafka.ConnectionSpec, error) {
	return kafka.ConnectionSpec{ID: id, Kind: s.kind}, nil
}

func demoOpener(t *testing.T) SpecClientOpener {
	t.Helper()
	settings := kafka.DefaultDemoSettings()
	settings.Backlog, settings.Interval = 3, time.Hour
	pool := kafka.NewDemoClusterPool(settings)
	t.Cleanup(pool.Close)
	factory := kafka.NewDemoClientFactory(pool, kafka.DefaultPurposeDefaults())
	return SpecClientOpener{Specs: demoSpecs{kind: kafka.ClusterDemo}, Kinds: map[kafka.ClusterKind]KindOptions{kafka.ClusterDemo: SandboxKindOptions{Inner: factory}}}
}

func demoManager(t *testing.T, opener SpecClientOpener, specs SpecLookup) *Manager {
	t.Helper()
	routes := fakeRoutes{
		route:    RouteInfo{ID: "r1", Name: "order-events", TeamID: "t1", ActiveVersionID: "v1"},
		versions: []VersionConfig{{ID: "v1", No: 3, Config: json.RawMessage(demoRouteConfig)}},
	}
	m, err := NewAssembly(AssemblyDeps{
		Settings: Settings{StateBackend: StateBackendMemory, Executor: ExecutorInProcess, Scripts: ScriptsNone, DemoSeed: DemoSeedOn},
		Routes:   routes, Clients: opener, Specs: specs, Compiler: compile.NewDefaultCompiler(), Scripts: NoScriptRuntimes{},
		Salts: ctxhelpers.StaticSaltProvider{}, Audit: NoAuditTrail{}, Log: slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	}).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { w.t.Log(string(p)); return len(p), nil }

func TestDemoRouteListsSeededDeadLettersOnce(t *testing.T) {
	opener := demoOpener(t)
	m := demoManager(t, opener, opener.Specs)
	ctx := context.Background()
	page, err := m.List(ctx, "r1", Query{})
	if err != nil || len(page.Items) != 12 {
		t.Fatalf("list %v %d", err, len(page.Items))
	}
	kinds := map[record.ErrorType]int{}
	for _, it := range page.Items {
		kinds[it.ErrorType]++
		if it.VersionNo != 3 || it.Source.Topic != "order-events" || it.RouteName != "order-events" {
			t.Fatalf("row %+v", it)
		}
	}
	if kinds[record.ErrorStep] != 5 || kinds[record.ErrorOutputInvalid] != 3 || kinds[record.ErrorTimeout] != 2 || kinds[record.ErrorDecode] != 2 {
		t.Fatalf("mix %v", kinds)
	}
	if page.Items[len(page.Items)-1].StepLabel != "Step 1 · Rename field" {
		t.Fatalf("oldest step label %q", page.Items[len(page.Items)-1].StepLabel)
	}
	again := demoManager(t, opener, opener.Specs)
	if p2, err := again.List(ctx, "r1", Query{}); err != nil || len(p2.Items) != 12 {
		t.Fatalf("second process view %v %d", err, len(p2.Items))
	}
}

func TestDemoDeadLetterRetrySucceedsThroughActiveVersion(t *testing.T) {
	opener := demoOpener(t)
	m := demoManager(t, opener, opener.Specs)
	ctx := context.Background()
	q := Query{ErrorTypes: []record.ErrorType{record.ErrorStep}}
	report, err := m.Act(ctx, ActionRetry, Actor{UserID: "u1"}, "r1", Selection{Match: &q})
	if err != nil || report.Counts()[OutcomeSent] != 5 {
		t.Fatalf("retry %v %+v", err, report.Counts())
	}
	left, _ := m.List(ctx, "r1", Query{})
	if len(left.Items) != 7 {
		t.Fatalf("left %d", len(left.Items))
	}
	detail, err := m.Get(ctx, Viewer{}, "r1", left.Items[len(left.Items)-1].ID)
	if err != nil || detail.Value.Encoding != EncodingJSON {
		t.Fatalf("detail %v %+v", err, detail.Value)
	}
}

func TestDemoSeedingSkipsRealClusters(t *testing.T) {
	_, seeds := newCluster(t, "router.dlq.order-events")
	m := demoManager(t, SpecClientOpener{Specs: demoSpecs{kind: kafka.ClusterKafka}, Kinds: map[kafka.ClusterKind]KindOptions{kafka.ClusterKafka: seedOptions{seeds}}}, demoSpecs{kind: kafka.ClusterKafka})
	page, err := m.List(context.Background(), "r1", Query{})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("real cluster seeded %v %d", err, len(page.Items))
	}
}

func TestDemoDeadLetterSetShape(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	recs := (DemoDeadLetterSet{}).Records(DemoTarget{RouteID: "r1", VersionNo: 2, Branch: "main", Step: "s1", SourceTopic: "order-events"}, now)
	if len(recs) != 12 || !recs[0].Failure.FailedAt.Equal(now.Add(-2*time.Hour)) || !recs[11].Failure.FailedAt.Before(now) {
		t.Fatalf("set %d %v", len(recs), recs[0].Failure.FailedAt)
	}
	for _, r := range recs {
		back, err := record.Codec{}.Decode(record.Codec{}.Encode(r))
		if err != nil || back.Failure.ErrorType != r.Failure.ErrorType || back.Source != r.Source {
			t.Fatalf("round trip %v %+v", err, back.Failure)
		}
	}
	if json.Valid(recs[11].Value) || !json.Valid(recs[0].Value) || recs[0].Failure.Step != "s1" || recs[5].Failure.Step != "" {
		t.Fatalf("payloads %s / %s", recs[0].Value, recs[11].Value)
	}
	if (DemoSeederRegistry{}).Build(DemoSeedOn) == nil {
		t.Fatal("nil seeder")
	}
}

type seedOptions struct{ seeds SeedClientOpener }

func (o seedOptions) Options(context.Context, kafka.ConnectionSpec, kafka.ClientPurpose) ([]kgo.Opt, error) {
	return []kgo.Opt{kgo.SeedBrokers(o.seeds.Seeds...)}, nil
}
