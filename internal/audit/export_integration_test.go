package audit_test

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/audit"
)

// webhookCollector is an httptest SIEM endpoint that can be switched down (503) and counts received lines.
type webhookCollector struct {
	mu    sync.Mutex
	down  bool
	lines int
}

func (c *webhookCollector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.down {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	sc := bufio.NewScanner(r.Body)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<20)
	for sc.Scan() {
		c.lines++
	}
	w.WriteHeader(http.StatusAccepted)
}

func (c *webhookCollector) set(down bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.down = down
}

// TestExportJobResumesFromCheckpointAfterWebhookFailure (AC-213): entries written while the SIEM is down are
// delivered exactly once after it recovers, the checkpoint never moves on failure and keeps the last error.
func TestExportJobResumesFromCheckpointAfterWebhookFailure(t *testing.T) {
	f := newAuditFixture(t)
	ctx := context.Background()
	collector := &webhookCollector{}
	srv := httptest.NewServer(collector)
	defer srv.Close()
	sink, err := audit.NewExportSinkRegistry().Build(audit.ExportSinkWebhook, audit.ExportSinkDeps{Target: srv.URL, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	reader, cps := audit.NewPostgresReader(f.sb.DB), audit.NewPostgresCheckpointStore(f.sb.DB)
	job := &audit.AuditExportJob{Sink: sink, Checkpoints: cps, Source: reader, Metrics: audit.NewPromExportMetrics(),
		Interval: time.Second, Batch: 3, Backoff: &audit.Backoff{Base: time.Second, Max: time.Minute}, Now: time.Now,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	f.seed(t, 5)
	drain(t, job)
	if collector.lines != 5 {
		t.Fatalf("first drain delivered %d", collector.lines)
	}
	collector.set(true)
	f.seed(t, 4)
	if _, err := job.Tick(ctx); err == nil {
		t.Fatal("down webhook must fail")
	}
	cp, _ := cps.Load(ctx, audit.ExportSinkWebhook)
	if cp.ExportedTotal != 5 || cp.LastError == "" {
		t.Fatalf("failure moved the checkpoint or lost the error: %+v", cp)
	}
	status, err := audit.ExportMonitor{Settings: audit.ExportSettings{Sink: audit.ExportSinkWebhook, Target: srv.URL},
		Checkpoints: cps, Source: reader}.Status(ctx)
	if err != nil || status.LagRows != 4 || status.Checkpoint.LastError == "" {
		t.Fatalf("status: %+v %v", status, err)
	}
	collector.set(false)
	drain(t, job)
	cp, _ = cps.Load(ctx, audit.ExportSinkWebhook)
	if collector.lines != 9 || cp.ExportedTotal != 9 || cp.LastError != "" {
		t.Fatalf("resume: lines=%d cp=%+v", collector.lines, cp)
	}
}

// drain ticks until the job reports an idle wait.
func drain(t *testing.T, job *audit.AuditExportJob) {
	t.Helper()
	for range 20 {
		wait, err := job.Tick(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if wait == job.Interval {
			return
		}
	}
	t.Fatal("job never went idle")
}

// seed appends n plain entries through the recorder.
func (f *auditFixture) seed(t *testing.T, n int) {
	t.Helper()
	for i := range n {
		e := audit.Entry{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: audit.ActionRouteCreated, Object: audit.ObjectRoute,
			ObjectID: strconv.Itoa(i), ObjectLabel: "export-test"}
		if err := f.recorder.Record(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
}

// PerfBudget is the audit query latency target (requirements: p95 < 500 ms).
const PerfBudget = 500 * time.Millisecond

// EnvAuditPerfRows overrides how many rows the query benchmark seeds (default 1,000,000).
const EnvAuditPerfRows = "ROUTER_TEST_AUDIT_PERF_ROWS"

// TestAuditQueryP95OnSeededRows (AC-213) seeds 1 M rows and measures the audit screen's query shapes.
func TestAuditQueryP95OnSeededRows(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds 1 M rows")
	}
	f := newAuditFixture(t)
	ctx := context.Background()
	rows := 1_000_000
	if v, err := strconv.Atoi(os.Getenv(EnvAuditPerfRows)); err == nil && v > 0 {
		rows = v
	}
	_, err := f.sb.DB.Pool().Exec(ctx, `INSERT INTO audit_log (action, object_type, object_id, object_label, actor_kind, sensitive, at, ip)
		SELECT (ARRAY['route.created','route.deployed','route.paused','connection.updated','sample.viewed','sample.viewed_raw',
			'dlq.retried','role_binding.set','auth.login','transform.saved'])[1 + i % 10], 'route', 'obj-' || i,
			'route-' || (i % 5000), 'system', i % 50 = 0, now() - make_interval(secs => i), '10.0.0.1'
		FROM generate_series(1, $1) i`, rows)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.sb.DB.Pool().Exec(ctx, `ANALYZE audit_log`); err != nil {
		t.Fatal(err)
	}
	m := audit.NewManager(audit.NewPostgresReader(f.sb.DB), audit.NewCatalog())
	from := time.Now().Add(-48 * time.Hour)
	shapes := map[string]audit.Filter{
		"newest":         {},
		"sensitive only": {SensitiveOnly: true},
		"one action":     {Actions: []audit.Action{audit.ActionRouteDeployed}},
		"time range":     {From: &from},
		"text frequent":  {Text: "route-42"},
		"text rare":      {Text: "obj-999999"},
	}
	var all []time.Duration
	for name, filter := range shapes {
		for range 5 {
			start := time.Now()
			if _, err := m.Query(ctx, filter, audit.Page{Limit: 50}); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			d := time.Since(start)
			all = append(all, d)
			t.Logf("%-15s %s", name, d)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	p95 := all[len(all)*95/100]
	t.Logf("p95 over %d queries on %d rows: %s", len(all), rows, p95)
	if p95 > PerfBudget {
		t.Fatalf("p95 %s exceeds %s", p95, PerfBudget)
	}
}
