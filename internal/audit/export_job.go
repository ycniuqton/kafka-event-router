package audit

import (
	"context"
	"log/slog"
	"net/url"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
)

// Checkpoint is the continuous-export position of one sink.
type Checkpoint struct {
	Sink          ExportSinkKind `json:"sink"`
	LastID        int64          `json:"lastId,string"`
	ExportedTotal int64          `json:"exportedTotal"`
	LastError     string         `json:"lastError,omitempty"`
	LastAttemptAt *time.Time     `json:"lastAttemptAt"`
	UpdatedAt     *time.Time     `json:"updatedAt"`
}

// CheckpointStore persists export positions (Postgres audit_export_checkpoints; fakes in tests).
type CheckpointStore interface {
	Load(ctx context.Context, sink ExportSinkKind) (Checkpoint, error)
	Advance(ctx context.Context, sink ExportSinkKind, lastID int64, count int, at time.Time) error
	Fail(ctx context.Context, sink ExportSinkKind, reason string, at time.Time) error
}

// BatchSource reads entries after a checkpoint in id order and counts what is still to export.
type BatchSource interface {
	After(ctx context.Context, lastID int64, limit int) ([]LogEntry, error)
	Lag(ctx context.Context, lastID int64) (int64, error)
}

// ExportMetrics observes the export (router_audit_exported_total, _errors_total, _lag_rows).
type ExportMetrics interface {
	Exported(sink ExportSinkKind, n int)
	Failed(sink ExportSinkKind)
	Lag(sink ExportSinkKind, rows int64)
}

// LeaderGate tells whether this API instance runs leader-only jobs (route-runtime's scheduler leadership).
type LeaderGate interface {
	IsLeader() bool
}

// Backoff doubles the wait after each failed delivery, from Base up to Max; a success resets it.
type Backoff struct {
	Base     time.Duration
	Max      time.Duration
	failures int
}

// Fail records a failure and returns the wait before the next attempt.
func (b *Backoff) Fail() time.Duration {
	b.failures++
	d := b.Base
	for i := 1; i < b.failures && d < b.Max; i++ {
		d *= 2
	}
	return min(d, b.Max)
}

// Reset forgets earlier failures.
func (b *Backoff) Reset() {
	b.failures = 0
}

// ExportLog names the log messages of the export job.
type ExportLog string

const (
	LogExportFailed  ExportLog = "audit export: sink delivery failed, checkpoint kept"
	LogExportStopped ExportLog = "audit export: job stopped"
	AttrSink         ExportLog = "sink"
	AttrBackoff      ExportLog = "backoff"
)

// AuditExportJob ships new audit entries to the configured sink on the leader (design flow 5.9):
// at-least-once, checkpointed after each delivered batch, backing off while the sink fails.
type AuditExportJob struct {
	Leader      LeaderGate
	Sink        ExportSink
	Checkpoints CheckpointStore
	Source      BatchSource
	Metrics     ExportMetrics
	Interval    time.Duration
	Batch       int
	Backoff     *Backoff
	Now         func() time.Time
	Log         *slog.Logger
}

// Run ticks until ctx ends; followers only wait for leadership.
func (j *AuditExportJob) Run(ctx context.Context) {
	timer := time.NewTimer(j.Interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			j.Log.Info(string(LogExportStopped), string(AttrSink), j.Sink.Kind())
			return
		case <-timer.C:
			timer.Reset(j.next(ctx))
		}
	}
}

// next runs one tick on the leader and returns the wait before the following one.
func (j *AuditExportJob) next(ctx context.Context) time.Duration {
	if !j.Leader.IsLeader() {
		return j.Interval
	}
	wait, err := j.Tick(ctx)
	if err != nil {
		j.Log.Warn(string(LogExportFailed), string(AttrSink), j.Sink.Kind(), string(AttrBackoff), wait.String(),
			string(logging.AttrError), err)
	}
	return wait
}

// Tick delivers at most one batch; a full batch asks for an immediate next tick (catch-up).
func (j *AuditExportJob) Tick(ctx context.Context) (time.Duration, error) {
	kind := j.Sink.Kind()
	cp, err := j.Checkpoints.Load(ctx, kind)
	if err != nil {
		return j.Backoff.Fail(), err
	}
	batch, err := j.Source.After(ctx, cp.LastID, j.Batch)
	if err != nil {
		return j.Backoff.Fail(), err
	}
	if len(batch) == 0 {
		j.Metrics.Lag(kind, 0)
		return j.Interval, nil
	}
	if err := j.Sink.Send(ctx, batch); err != nil {
		return j.failed(ctx, cp, err), err
	}
	last := batch[len(batch)-1].ID
	if err := j.Checkpoints.Advance(ctx, kind, last, len(batch), j.Now()); err != nil {
		return j.Backoff.Fail(), err
	}
	j.Backoff.Reset()
	j.Metrics.Exported(kind, len(batch))
	j.observeLag(ctx, last)
	if len(batch) >= j.Batch {
		return 0, nil
	}
	return j.Interval, nil
}

// failed keeps the checkpoint, stores the reason for the status view and returns the backoff.
func (j *AuditExportJob) failed(ctx context.Context, cp Checkpoint, cause error) time.Duration {
	j.Metrics.Failed(j.Sink.Kind())
	_ = j.Checkpoints.Fail(ctx, j.Sink.Kind(), cause.Error(), j.Now())
	j.observeLag(ctx, cp.LastID)
	return j.Backoff.Fail()
}

func (j *AuditExportJob) observeLag(ctx context.Context, lastID int64) {
	if lag, err := j.Source.Lag(ctx, lastID); err == nil {
		j.Metrics.Lag(j.Sink.Kind(), lag)
	}
}

// ExportStatus is what Settings → Workspace shows about the continuous export.
type ExportStatus struct {
	Enabled    bool           `json:"enabled"`
	Sink       ExportSinkKind `json:"sink"`
	Target     string         `json:"target"`
	Checkpoint *Checkpoint    `json:"checkpoint"`
	LagRows    int64          `json:"lagRows"`
}

// ExportMonitor reports the export's configuration and progress (any instance can answer).
type ExportMonitor struct {
	Settings    ExportSettings
	Checkpoints CheckpointStore
	Source      BatchSource
}

// Status returns the current export status; a disabled export has no checkpoint.
func (m ExportMonitor) Status(ctx context.Context) (ExportStatus, error) {
	out := ExportStatus{Enabled: m.Settings.Enabled(), Sink: m.Settings.Sink, Target: TargetRedactor{}.Redact(m.Settings.Target)}
	if !out.Enabled {
		return out, nil
	}
	cp, err := m.Checkpoints.Load(ctx, m.Settings.Sink)
	if err != nil {
		return out, err
	}
	out.Checkpoint = &cp
	out.LagRows, err = m.Source.Lag(ctx, cp.LastID)
	return out, err
}

// TargetRedactor hides credentials and query strings of sink targets before they are shown.
type TargetRedactor struct{}

// Redact returns scheme://host/path of target.
func (TargetRedactor) Redact(target string) string {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return ""
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
}
