package lookup

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// MetricName names the lookup metrics.
type MetricName string

const (
	MetricRequests MetricName = "router_lookup_requests_total"
	MetricDuration MetricName = "router_lookup_duration_seconds"
	MetricCache    MetricName = "router_lookup_cache_total"
	MetricBreaker  MetricName = "router_lookup_breaker_state"
)

// MetricLabel names metric labels.
type MetricLabel string

const (
	LabelSource MetricLabel = "source"
	LabelResult MetricLabel = "result"
)

// RequestResult is the `result` label of router_lookup_requests_total.
type RequestResult string

const (
	ResultHit         RequestResult = "hit"
	ResultMiss        RequestResult = "miss"
	ResultError       RequestResult = "error"
	ResultTimeout     RequestResult = "timeout"
	ResultCircuitOpen RequestResult = "circuit_open"
	ResultRateLimited RequestResult = "rate_limited"
)

// DurationBuckets are the latency buckets (1 ms … 5 s).
var DurationBuckets = []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

// Observer receives lookup observations (Prometheus in services, no-op in tools and tests).
type Observer interface {
	Request(source string, result RequestResult, d time.Duration)
	Cache(source string, ev ctxhelpers.SourceEvent)
	Breaker(source string, state BreakerState)
}

// NoopObserver discards observations.
type NoopObserver struct{}

func (NoopObserver) Request(string, RequestResult, time.Duration) {}
func (NoopObserver) Cache(string, ctxhelpers.SourceEvent)         {}
func (NoopObserver) Breaker(string, BreakerState)                 {}

// PromObserver records the router_lookup_* metrics.
type PromObserver struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	cache    *prometheus.CounterVec
	breaker  *prometheus.GaugeVec
}

// NewPromObserver registers the metrics on reg (nil = unregistered).
func NewPromObserver(reg prometheus.Registerer) *PromObserver {
	source, result := string(LabelSource), string(LabelResult)
	o := &PromObserver{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricRequests), Help: "Lookup calls by source and result."}, []string{source, result}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: string(MetricDuration), Help: "Lookup call latency.", Buckets: DurationBuckets}, []string{source}),
		cache:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricCache), Help: "Lookup cache lookups by result (hit ratio = hit/(hit+miss))."}, []string{source, result}),
		breaker:  prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: string(MetricBreaker), Help: "Circuit breaker state: 0 closed, 1 half-open, 2 open."}, []string{source}),
	}
	if reg != nil {
		reg.MustRegister(o.requests, o.duration, o.cache, o.breaker)
	}
	return o
}

// Request implements Observer.
func (o *PromObserver) Request(source string, result RequestResult, d time.Duration) {
	o.requests.WithLabelValues(source, string(result)).Inc()
	o.duration.WithLabelValues(source).Observe(d.Seconds())
}

// Cache implements Observer.
func (o *PromObserver) Cache(source string, ev ctxhelpers.SourceEvent) {
	o.cache.WithLabelValues(source, string(ev)).Inc()
}

// Breaker implements Observer.
func (o *PromObserver) Breaker(source string, state BreakerState) {
	o.breaker.WithLabelValues(source).Set(float64(state))
}
