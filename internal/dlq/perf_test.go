package dlq

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// PerfBudget is the NFR-7 regression threshold of the list benchmark.
type PerfBudget time.Duration

const (
	ListP95Budget  PerfBudget = PerfBudget(1500 * time.Millisecond)
	perfEnv                   = "ROUTER_DLQ_PERF_RECORDS"
	perfDefault               = 1_000_000
	perfPartitions            = 6
	perfSpan                  = 7 * 24 * time.Hour
	perfBatch                 = 20_000
	perfCalls                 = 20
)

// perfRecords returns the topic size: ROUTER_DLQ_PERF_RECORDS, else 1M (skipped with -short).
func perfRecords(t testing.TB) int {
	if v, err := strconv.Atoi(os.Getenv(perfEnv)); err == nil && v > 0 {
		return v
	}
	if testing.Short() {
		t.Skip("1M-record DLQ benchmark skipped with -short (set " + perfEnv + " to size it)")
	}
	return perfDefault
}

// perfCluster fills a 6-partition DLQ topic with n records spread evenly over the 7-day retention, ending now.
func perfCluster(t testing.TB, n int, now time.Time) SeedClientOpener {
	t.Helper()
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(perfPartitions, testDLQTopic))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	opener := SeedClientOpener{Seeds: c.ListenAddrs()}
	cl, err := opener.Open(context.Background(), "c1", "", kgo.RecordPartitioner(kgo.ManualPartitioner()), kgo.ProducerBatchMaxBytes(16<<20), kgo.MaxBufferedRecords(perfBatch*2))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	step := perfSpan / time.Duration(n)
	for start := 0; start < n; start += perfBatch {
		batch := make([]*kgo.Record, 0, perfBatch)
		for i := start; i < min(start+perfBatch, n); i++ {
			batch = append(batch, perfRecord(i, now.Add(-perfSpan+time.Duration(i)*step)))
		}
		if err := cl.ProduceSync(context.Background(), batch...).FirstErr(); err != nil {
			t.Fatal(err)
		}
	}
	return opener
}

func perfRecord(i int, at time.Time) *kgo.Record {
	rec := failure(i, record.ErrorTypes[i%len(record.ErrorTypes)])
	rec.Failure.FailedAt = at
	raw := record.Codec{}.Encode(rec)
	hs := make([]kgo.RecordHeader, 0, len(raw.Headers))
	for _, h := range raw.Headers {
		hs = append(hs, kgo.RecordHeader{Key: h.Key, Value: h.Value})
	}
	return &kgo.Record{Topic: testDLQTopic, Partition: int32(i % perfPartitions), Key: raw.Key, Value: raw.Value, Headers: hs, Timestamp: at}
}

func perfManager(t testing.TB, opener SeedClientOpener) *Manager {
	t.Helper()
	routes := fakeRoutes{
		route:    RouteInfo{ID: "r1", Name: "orders", TeamID: "t1", ActiveVersionID: "v1"},
		versions: []VersionConfig{{ID: "v1", No: 1, Config: json.RawMessage(e2eRouteConfig)}},
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

// TestListP95OnMillionRecordTopic is the NFR-7 regression gate (DL-208): list pages of a 1M-record DLQ
// topic with a time filter (last hour, a day back, plus the next page) must stay under 1.5 s at p95.
func TestListP95OnMillionRecordTopic(t *testing.T) {
	n, now := perfRecords(t), time.Now()
	m := perfManager(t, perfCluster(t, n, now))
	windows := []Query{
		{Since: now.Add(-time.Hour)},
		{Since: now.Add(-25 * time.Hour), Until: now.Add(-24 * time.Hour), ErrorTypes: []record.ErrorType{record.ErrorStep}},
		{Since: now.Add(-3 * 24 * time.Hour), Limit: int(MaxPage)},
	}
	took := make([]time.Duration, 0, perfCalls)
	for i := 0; i < perfCalls; i++ {
		q := windows[i%len(windows)]
		start := time.Now()
		page, err := m.List(context.Background(), "r1", q)
		took = append(took, time.Since(start))
		if err != nil || len(page.Items) == 0 {
			t.Fatalf("list %d: %v (%d items)", i, err, len(page.Items))
		}
		if page.NextCursor != nil && i%len(windows) == 0 {
			q.Cursor = *page.NextCursor
			start = time.Now()
			if _, err := m.List(context.Background(), "r1", q); err != nil {
				t.Fatal(err)
			}
			took = append(took, time.Since(start))
		}
	}
	sort.Slice(took, func(i, j int) bool { return took[i] < took[j] })
	p95 := took[(len(took)*95+99)/100-1]
	t.Logf("records=%d calls=%d p50=%s p95=%s max=%s", n, len(took), took[len(took)/2], p95, took[len(took)-1])
	if p95 > time.Duration(ListP95Budget) {
		t.Fatalf("list p95 %s over budget %s", p95, time.Duration(ListP95Budget))
	}
}

// BenchmarkListTimeFilter measures one time-filtered list page (size via ROUTER_DLQ_PERF_RECORDS).
func BenchmarkListTimeFilter(b *testing.B) {
	n, now := perfRecords(b), time.Now()
	m := perfManager(b, perfCluster(b, n, now))
	q := Query{Since: now.Add(-time.Hour)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := m.List(context.Background(), "r1", q); err != nil {
			b.Fatal(err)
		}
	}
}
