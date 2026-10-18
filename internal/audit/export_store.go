package audit

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/store"
)

// PostgresCheckpointStore keeps export positions in audit_export_checkpoints (one row per sink kind).
type PostgresCheckpointStore struct {
	db *store.DB
}

// NewPostgresCheckpointStore builds the store.
func NewPostgresCheckpointStore(db *store.DB) *PostgresCheckpointStore {
	return &PostgresCheckpointStore{db: db}
}

// Load returns the checkpoint of sink, or a zero checkpoint before the first delivery.
func (s *PostgresCheckpointStore) Load(ctx context.Context, sink ExportSinkKind) (Checkpoint, error) {
	cp := Checkpoint{Sink: sink}
	var lastErr *string
	err := s.db.Q(ctx).QueryRow(ctx, `SELECT last_id, exported_total, last_error, last_attempt_at, updated_at
		FROM audit_export_checkpoints WHERE sink = $1`, string(sink)).
		Scan(&cp.LastID, &cp.ExportedTotal, &lastErr, &cp.LastAttemptAt, &cp.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return cp, nil
	}
	if lastErr != nil {
		cp.LastError = *lastErr
	}
	return cp, s.db.Translate(err)
}

// Advance moves the checkpoint forward after a delivered batch and clears the last error.
func (s *PostgresCheckpointStore) Advance(ctx context.Context, sink ExportSinkKind, lastID int64, count int, at time.Time) error {
	_, err := s.db.Q(ctx).Exec(ctx, `INSERT INTO audit_export_checkpoints (sink, last_id, exported_total, last_attempt_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (sink) DO UPDATE SET last_id = GREATEST(audit_export_checkpoints.last_id, EXCLUDED.last_id),
			exported_total = audit_export_checkpoints.exported_total + EXCLUDED.exported_total,
			last_error = NULL, last_attempt_at = EXCLUDED.last_attempt_at, updated_at = EXCLUDED.updated_at`,
		string(sink), lastID, count, at)
	return s.db.Translate(err)
}

// Fail records the reason of a failed delivery without moving the checkpoint.
func (s *PostgresCheckpointStore) Fail(ctx context.Context, sink ExportSinkKind, reason string, at time.Time) error {
	_, err := s.db.Q(ctx).Exec(ctx, `INSERT INTO audit_export_checkpoints (sink, last_error, last_attempt_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (sink) DO UPDATE SET last_error = EXCLUDED.last_error, last_attempt_at = EXCLUDED.last_attempt_at`,
		string(sink), reason, at)
	return s.db.Translate(err)
}

// After returns up to limit entries with id > lastID in id order (the export's delivery order).
func (r *PostgresReader) After(ctx context.Context, lastID int64, limit int) ([]LogEntry, error) {
	rows, err := r.db.Q(ctx).Query(ctx, r.selectSQL+` WHERE a.id > $1 ORDER BY a.id LIMIT $2`, lastID, limit)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		e, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, r.db.Translate(rows.Err())
}

// Lag counts the entries not exported yet.
func (r *PostgresReader) Lag(ctx context.Context, lastID int64) (int64, error) {
	var n int64
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE id > $1`, lastID).Scan(&n)
	return n, r.db.Translate(err)
}

// ExportMetricName names the export metrics (design §8).
type ExportMetricName string

const (
	MetricExportedTotal   ExportMetricName = "router_audit_exported_total"
	MetricExportErrors    ExportMetricName = "router_audit_export_errors_total"
	MetricExportLag       ExportMetricName = "router_audit_export_lag_rows"
	ExportMetricLabelSink ExportMetricName = "sink"
)

// PromExportMetrics implements ExportMetrics with Prometheus collectors.
type PromExportMetrics struct {
	exported *prometheus.CounterVec
	errors   *prometheus.CounterVec
	lag      *prometheus.GaugeVec
}

// NewPromExportMetrics builds the collectors (register them with Collectors).
func NewPromExportMetrics() *PromExportMetrics {
	labels := []string{string(ExportMetricLabelSink)}
	return &PromExportMetrics{
		exported: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricExportedTotal),
			Help: "Audit entries delivered to the SIEM export sink."}, labels),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricExportErrors),
			Help: "Failed audit export deliveries."}, labels),
		lag: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: string(MetricExportLag),
			Help: "Audit entries written but not exported yet."}, labels),
	}
}

func (m *PromExportMetrics) Exported(sink ExportSinkKind, n int) {
	m.exported.WithLabelValues(string(sink)).Add(float64(n))
}
func (m *PromExportMetrics) Failed(sink ExportSinkKind) { m.errors.WithLabelValues(string(sink)).Inc() }
func (m *PromExportMetrics) Lag(sink ExportSinkKind, rows int64) {
	m.lag.WithLabelValues(string(sink)).Set(float64(rows))
}

// Collectors returns the collectors to register.
func (m *PromExportMetrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{m.exported, m.errors, m.lag}
}
