package approvals

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/platform/metrics"
)

// MetricName names the approval metrics (design §8).
type MetricName string

const (
	MetricPending         MetricName = "approvals_pending"
	MetricDecisionSeconds MetricName = "approval_decision_seconds"
)

// MetricLabel names metric labels.
type MetricLabel string

const (
	LabelTeam    MetricLabel = "team"
	LabelOutcome MetricLabel = "outcome"
)

// DecisionBuckets are the latency histogram buckets: 1 min × 4^n, up to about 11 days.
type DecisionBuckets float64

const (
	DecisionBucketStart  DecisionBuckets = 60
	DecisionBucketFactor DecisionBuckets = 4
	DecisionBucketCount  DecisionBuckets = 8
)

// ScrapeTimeout bounds the pending-count query run on each scrape.
type ScrapeTimeout time.Duration

const PendingScrapeTimeout ScrapeTimeout = ScrapeTimeout(2 * time.Second)

// Metrics records decision latency and exposes the pending gauge.
type Metrics struct {
	decision *prometheus.HistogramVec
	pending  *PendingCollector
}

// NewMetrics registers router_approvals_pending and router_approval_decision_seconds on reg (nil: unregistered).
func NewMetrics(reg prometheus.Registerer, st Store) *Metrics {
	m := &Metrics{
		decision: prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: string(metrics.RouterNamespace), Name: string(MetricDecisionSeconds),
			Help: "Time from approval request to decision.", Buckets: prometheus.ExponentialBuckets(float64(DecisionBucketStart), float64(DecisionBucketFactor), int(DecisionBucketCount))}, []string{string(LabelOutcome)}),
		pending: &PendingCollector{store: st, desc: prometheus.NewDesc(prometheus.BuildFQName(string(metrics.RouterNamespace), "", string(MetricPending)),
			"Approval requests waiting for a decision, per team.", []string{string(LabelTeam)}, nil)},
	}
	if reg != nil {
		reg.MustRegister(m.decision, m.pending)
	}
	return m
}

// Collectors returns the metrics to register on the service registry.
func (m *Metrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{m.decision, m.pending}
}

// Decided observes the time a request waited for its decision.
func (m *Metrics) Decided(a Approval, outcome ApprovalStatus, now time.Time) {
	if m == nil {
		return
	}
	m.decision.WithLabelValues(string(outcome)).Observe(now.Sub(a.RequestedAt).Seconds())
}

// PendingCollector reads pending counts per team from the store at scrape time.
type PendingCollector struct {
	store Store
	desc  *prometheus.Desc
}

// Describe implements prometheus.Collector.
func (c *PendingCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

// Collect implements prometheus.Collector (a failed query exports nothing).
func (c *PendingCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(PendingScrapeTimeout))
	defer cancel()
	counts, err := c.store.PendingByTeam(ctx)
	if err != nil {
		return
	}
	for team, n := range counts {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(n), team.String())
	}
}
