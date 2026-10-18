package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/dlq"
)

type captureRecorder struct {
	hook    audit.EntryHook
	entries []audit.Entry
}

func (c *captureRecorder) Record(ctx context.Context, e audit.Entry) error {
	e, err := c.hook.Before(ctx, e)
	if err != nil {
		return err
	}
	c.entries = append(c.entries, e)
	return nil
}

type routeLookup struct{ err error }

func (r routeLookup) Route(_ context.Context, id string) (dlq.RouteInfo, error) {
	return dlq.RouteInfo{ID: id, Name: "order-events", TeamID: "6f1c1b0e-1d8a-4c56-8a5c-0d6f3d5c2a11"}, r.err
}

type entrySlice []audit.LogEntry

func (s entrySlice) Each(_ context.Context, visit func(audit.LogEntry) error) error {
	for _, e := range s {
		if err := visit(e); err != nil {
			return err
		}
	}
	return nil
}

func dlqCatalog() *audit.Catalog {
	c := audit.NewCatalog()
	for a, meta := range dlq.CatalogActions {
		c.Register(a, meta)
	}
	return c
}

func TestEveryDeadLetterActionIsCatalogued(t *testing.T) {
	c := dlqCatalog()
	for _, a := range dlq.AuditActions {
		meta, ok := c.Describe(a)
		if !ok || meta.Category == "" || !strings.HasPrefix(string(a), "dlq.") {
			t.Fatalf("%s not catalogued: %+v", a, meta)
		}
	}
}

func TestJobEndIsAuditedAsSystemActorAndExported(t *testing.T) {
	c := dlqCatalog()
	rec := &captureRecorder{hook: audit.NewCatalogHook(c)}
	actor := audit.Actor{Kind: audit.ActorSystem, Name: string(dlq.JobRunnerActor)}
	n := AuditNotifier{Trail: dlq.RecorderAuditTrail{Recorder: rec, Actor: actor}, Routes: routeLookup{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	job := Job{ID: "j1", RouteID: "r1", Action: dlq.ActionRetry, Status: StatusSucceeded, Processed: 620, RequestedBy: "u1",
		Counts: map[dlq.Outcome]int{dlq.OutcomeSent: 600, dlq.OutcomeFailed: 20}}
	n.JobEnded(context.Background(), job)
	n = AuditNotifier{Trail: n.Trail, Routes: routeLookup{err: errors.New("gone")}, Log: n.Log}
	n.JobEnded(context.Background(), Job{ID: "j2", RouteID: "r2", RouteName: "old", Action: dlq.ActionDiscard, Status: StatusFailed, Error: "scan failed"})
	if len(rec.entries) != 2 {
		t.Fatalf("entries %d", len(rec.entries))
	}
	e := rec.entries[0]
	if e.Action != dlq.ActionDLQJobFinished || e.Object != audit.ObjectDLQ || e.ObjectLabel != "order-events" || e.TeamID == nil || e.Actor.Kind != audit.ActorSystem {
		t.Fatalf("entry %+v", e)
	}
	if rec.entries[1].ObjectLabel != "old" || !strings.Contains(string(rec.entries[1].Diff), `"status":"failed"`) {
		t.Fatalf("fallback entry %+v %s", rec.entries[1], rec.entries[1].Diff)
	}
	assertExportFormats(t, c, rec.entries)
}

// assertExportFormats renders entries the way GET /audit-log/export and the SIEM stream ship them.
func assertExportFormats(t *testing.T, c *audit.Catalog, entries []audit.Entry) {
	t.Helper()
	logs := entrySlice{}
	for i, e := range entries {
		meta, _ := c.Describe(e.Action)
		logs = append(logs, audit.LogEntry{ID: int64(i + 1), At: time.Now(), Actor: audit.LogActor{Kind: e.Actor.Kind},
			Action: e.Action, Category: meta.Category, ObjectType: e.Object, ObjectID: e.ObjectID, ObjectLabel: e.ObjectLabel, TeamID: e.TeamID, Diff: e.Diff})
	}
	var jsonl, csv bytes.Buffer
	if err := (audit.JSONLExporter{}).Write(context.Background(), &jsonl, logs); err != nil {
		t.Fatal(err)
	}
	if err := (audit.CSVExporter{}).Write(context.Background(), &csv, logs); err != nil {
		t.Fatal(err)
	}
	first := strings.Split(strings.TrimSpace(jsonl.String()), "\n")[0]
	var parsed map[string]any
	if err := json.Unmarshal([]byte(first), &parsed); err != nil || parsed["action"] != "dlq.job_finished" || parsed["category"] != "change" || parsed["objectType"] != "dlq" {
		t.Fatalf("jsonl %v %s", err, first)
	}
	if !strings.Contains(csv.String(), "dlq.job_finished,change,dlq,r1,order-events") || !strings.Contains(csv.String(), ",system,dlq.job_finished") {
		t.Fatalf("csv %s", csv.String())
	}
	stream, _ := json.Marshal(audit.NewStreamRecord(1, time.Now(), entries[0]))
	if !strings.Contains(string(stream), `"counts":{"failed":20,"sent":600}`) {
		t.Fatalf("stream %s", stream)
	}
}
