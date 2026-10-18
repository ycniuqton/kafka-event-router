package audit_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/store"
)

// sliceEntries is an EntryIterator over fixed entries.
type sliceEntries []audit.LogEntry

func (s sliceEntries) Each(_ context.Context, visit func(audit.LogEntry) error) error {
	for _, e := range s {
		if err := visit(e); err != nil {
			return err
		}
	}
	return nil
}

func tricky() audit.LogEntry {
	actor := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	return audit.LogEntry{ID: 42, At: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC), Actor: audit.LogActor{ID: &actor,
		Name: `Hoa "the admin", L.`, Kind: audit.ActorUser}, Action: audit.ActionSampleViewedRaw, Category: audit.CategoryAccess,
		ObjectType: audit.ObjectSampleSession, ObjectID: "s1", ObjectLabel: "=HYPERLINK(\"x\")\nline two", IP: "10.0.0.1",
		Sensitive: true, Diff: json.RawMessage(`{"topics":["orders,eu"]}`)}
}

func TestCSVExporterEscapesQuotesNewlinesAndFormulas(t *testing.T) {
	var buf bytes.Buffer
	if err := (audit.CSVExporter{}).Write(context.Background(), &buf, sliceEntries{tricky()}); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v\n%s", err, buf.String())
	}
	if len(rows) != 2 || len(rows[0]) != len(audit.CSVColumns) || rows[0][0] != string(audit.ColID) {
		t.Fatalf("header/rows: %v", rows)
	}
	row := rows[1]
	if row[3] != `Hoa "the admin", L.` || row[9] != "'=HYPERLINK(\"x\")\nline two" || row[13] != "true" {
		t.Fatalf("row not escaped: %q", row)
	}
	if row[14] != `{"topics":["orders,eu"]}` || row[1] != "2026-10-14T09:00:00Z" {
		t.Fatalf("diff/time: %q", row)
	}
}

func TestJSONLExporterWritesOneEntryPerLine(t *testing.T) {
	var buf bytes.Buffer
	if err := (audit.JSONLExporter{}).Write(context.Background(), &buf, sliceEntries{tricky(), tricky()}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %s", len(lines), buf.String())
	}
	var back audit.LogEntry
	if err := json.Unmarshal([]byte(lines[0]), &back); err != nil || back.ObjectLabel != tricky().ObjectLabel || !back.Sensitive {
		t.Fatalf("round trip: %+v %v", back, err)
	}
}

func TestExporterRegistryRejectsUnknownFormat(t *testing.T) {
	reg := audit.NewExporterRegistry()
	if e, err := reg.Get(audit.FormatCSV); err != nil || e.ContentType() != string(audit.MediaCSV) {
		t.Fatalf("csv: %v", err)
	}
	var fe *audit.FilterError
	if _, err := reg.Get("xlsx"); !errors.As(err, &fe) || fe.Errors[0].Path != "/format" {
		t.Fatalf("want 422 at /format, got %v", err)
	}
}

// pagedReader serves n entries newest first in pages, honouring the cursor.
type pagedReader struct{ n int }

func (r pagedReader) Query(_ context.Context, _ audit.Filter, page audit.Page) ([]audit.LogEntry, *store.Cursor, error) {
	start := 0
	if page.After != nil {
		start = len(page.After.ID)
	}
	var out []audit.LogEntry
	for i := start; i < r.n && len(out) < page.Limit; i++ {
		out = append(out, audit.LogEntry{ID: int64(r.n - i)})
	}
	if start+len(out) >= r.n {
		return out, nil, nil
	}
	return out, &store.Cursor{ID: strings.Repeat("x", start+len(out))}, nil
}

func TestManagerExportWalksAllPages(t *testing.T) {
	m := audit.NewManager(pagedReader{n: 1203}, audit.NewCatalog())
	stream, err := m.Export(audit.Filter{}, audit.FormatJSONL)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := stream.WriteTo(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "\n"); n != 1203 {
		t.Fatalf("exported %d rows", n)
	}
	if name := stream.FileName(time.Date(2026, 10, 14, 9, 5, 0, 0, time.UTC)); name != "audit-log-20261014-090500.jsonl" {
		t.Fatalf("file name %s", name)
	}
}

func TestExportSinkRegistrySelectsConfiguredKind(t *testing.T) {
	reg := audit.NewExportSinkRegistry()
	cases := []struct {
		kind   audit.ExportSinkKind
		target string
		ok     bool
	}{
		{audit.ExportSinkNone, "", true},
		{audit.ExportSinkWebhook, "https://siem.example/ingest", true},
		{audit.ExportSinkWebhook, "udp://siem:514", false},
		{audit.ExportSinkSyslog, "udp://siem:514", true},
		{audit.ExportSinkSyslog, "tcp://siem:601", true},
		{audit.ExportSinkSyslog, "https://siem", false},
		{"kafka", "x", false},
	}
	for _, c := range cases {
		sink, err := reg.Build(c.kind, audit.ExportSinkDeps{Target: c.target, Timeout: time.Second, Host: "api-1"})
		if (err == nil) != c.ok {
			t.Fatalf("%s %s: err=%v", c.kind, c.target, err)
		}
		if err == nil && sink.Kind() != c.kind {
			t.Fatalf("built %s for %s", sink.Kind(), c.kind)
		}
	}
}

func TestWebhookExportSinkPostsNDJSONAndFailsOnNon2xx(t *testing.T) {
	var got []string
	status := http.StatusAccepted
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc := bufio.NewScanner(r.Body)
		for sc.Scan() {
			got = append(got, sc.Text())
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	sink, err := audit.NewWebhookExportSink(audit.ExportSinkDeps{Target: srv.URL, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Send(context.Background(), []audit.LogEntry{tricky(), tricky()}); err != nil || len(got) != 2 {
		t.Fatalf("send: %v lines=%d", err, len(got))
	}
	status = http.StatusServiceUnavailable
	if err := sink.Send(context.Background(), []audit.LogEntry{tricky()}); err == nil {
		t.Fatal("503 must fail the batch")
	}
}

func TestSyslogExportSinkFramesRFC5424OverTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	received := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		raw, _ := io.ReadAll(conn)
		received <- string(raw)
	}()
	sink, err := audit.NewSyslogExportSink(audit.ExportSinkDeps{Target: "tcp://" + ln.Addr().String(), Timeout: time.Second, Host: "api-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Send(context.Background(), []audit.LogEntry{tricky()}); err != nil {
		t.Fatal(err)
	}
	msg := <-received
	if !strings.Contains(msg, " <110>1 2026-10-14T09:00:00Z api-1 event-router - sample.viewed_raw - {") {
		t.Fatalf("not RFC 5424 with octet counting: %q", msg)
	}
}

// fakeCheckpoints is an in-memory CheckpointStore.
type fakeCheckpoints struct {
	mu sync.Mutex
	cp audit.Checkpoint
}

func (f *fakeCheckpoints) Load(_ context.Context, sink audit.ExportSinkKind) (audit.Checkpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := f.cp
	cp.Sink = sink
	return cp, nil
}

func (f *fakeCheckpoints) Advance(_ context.Context, _ audit.ExportSinkKind, last int64, n int, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cp.LastID, f.cp.ExportedTotal, f.cp.LastError = last, f.cp.ExportedTotal+int64(n), ""
	return nil
}

func (f *fakeCheckpoints) Fail(_ context.Context, _ audit.ExportSinkKind, reason string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cp.LastError = reason
	return nil
}

// idSource serves ids 1..n.
type idSource struct{ n int64 }

func (s idSource) After(_ context.Context, last int64, limit int) ([]audit.LogEntry, error) {
	var out []audit.LogEntry
	for id := last + 1; id <= s.n && len(out) < limit; id++ {
		out = append(out, audit.LogEntry{ID: id})
	}
	return out, nil
}

func (s idSource) Lag(_ context.Context, last int64) (int64, error) { return s.n - last, nil }

// flakySink fails while down and records what it delivered.
type flakySink struct {
	down bool
	got  []int64
}

func (s *flakySink) Kind() audit.ExportSinkKind { return audit.ExportSinkWebhook }
func (s *flakySink) Send(_ context.Context, batch []audit.LogEntry) error {
	if s.down {
		return errors.New("connection refused")
	}
	for _, e := range batch {
		s.got = append(s.got, e.ID)
	}
	return nil
}

type countingMetrics struct {
	exported, failed int
	lag              int64
}

func (m *countingMetrics) Exported(_ audit.ExportSinkKind, n int) { m.exported += n }
func (m *countingMetrics) Failed(audit.ExportSinkKind)            { m.failed++ }
func (m *countingMetrics) Lag(_ audit.ExportSinkKind, rows int64) { m.lag = rows }

func newJob(sink *flakySink, cps *fakeCheckpoints, n int64) (*audit.AuditExportJob, *countingMetrics) {
	m := &countingMetrics{}
	return &audit.AuditExportJob{Sink: sink, Checkpoints: cps, Source: idSource{n: n}, Metrics: m, Interval: 10 * time.Second,
		Batch: 4, Backoff: &audit.Backoff{Base: 10 * time.Second, Max: 5 * time.Minute}, Now: time.Now,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, m
}

func TestExportJobKeepsCheckpointAndBacksOffWhileSinkFails(t *testing.T) {
	sink, cps := &flakySink{down: true}, &fakeCheckpoints{}
	job, m := newJob(sink, cps, 6)
	var waits []time.Duration
	for range 7 {
		wait, err := job.Tick(context.Background())
		if err == nil {
			t.Fatal("down sink must fail")
		}
		waits = append(waits, wait)
	}
	want := []time.Duration{10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second, 160 * time.Second, 5 * time.Minute, 5 * time.Minute}
	for i := range want {
		if waits[i] != want[i] {
			t.Fatalf("backoff %v, want %v", waits, want)
		}
	}
	if cps.cp.LastID != 0 || cps.cp.LastError == "" || m.failed != 7 || m.lag != 6 {
		t.Fatalf("checkpoint moved or error not kept: %+v metrics=%+v", cps.cp, m)
	}
	sink.down = false
	if wait, err := job.Tick(context.Background()); err != nil || wait != 0 {
		t.Fatalf("full batch asks for an immediate tick: %v %v", wait, err)
	}
	if wait, err := job.Tick(context.Background()); err != nil || wait != 10*time.Second {
		t.Fatalf("partial batch waits one interval: %v %v", wait, err)
	}
	if len(sink.got) != 6 || sink.got[5] != 6 || cps.cp.LastID != 6 || cps.cp.LastError != "" || m.exported != 6 || m.lag != 0 {
		t.Fatalf("resume: got=%v cp=%+v metrics=%+v", sink.got, cps.cp, m)
	}
	if wait, err := job.Tick(context.Background()); err != nil || wait != 10*time.Second {
		t.Fatalf("idle tick: %v %v", wait, err)
	}
}

func TestExportSettingsLoaderDefaultsAndErrors(t *testing.T) {
	env := map[string]string{}
	loader := audit.ExportSettingsLoader{Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok }}
	s, err := loader.Load()
	if err != nil || s.Enabled() || s.Interval != 10*time.Second || s.Batch != 500 || s.MaxBackoff != 5*time.Minute {
		t.Fatalf("defaults: %+v %v", s, err)
	}
	env[string(audit.EnvExportSink)], env[string(audit.EnvExportTarget)] = "syslog", "udp://siem:514"
	if s, err = loader.Load(); err != nil || !s.Enabled() || s.Sink != audit.ExportSinkSyslog {
		t.Fatalf("configured: %+v %v", s, err)
	}
	env[string(audit.EnvExportBatch)] = "-1"
	if _, err := loader.Load(); err == nil {
		t.Fatal("negative batch must fail")
	}
}

func TestTargetRedactorHidesCredentials(t *testing.T) {
	if got := (audit.TargetRedactor{}).Redact("https://user:secret@siem.example/ingest?token=abc"); got != "https://siem.example/ingest" {
		t.Fatalf("redacted: %s", got)
	}
}
