package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// HTTPMetric names the API request metrics.
type HTTPMetric string

const (
	MetricAPIRequestsTotal   HTTPMetric = "api_requests_total"
	MetricAPIRequestDuration HTTPMetric = "api_request_duration_seconds"
)

// HTTPLabel names the labels of API request metrics.
type HTTPLabel string

const (
	LabelRoute  HTTPLabel = "route"
	LabelMethod HTTPLabel = "method"
	LabelStatus HTTPLabel = "status"
)

// UnmatchedRoute labels requests that matched no route (keeps cardinality bounded).
type UnmatchedRoute string

const RouteUnmatched UnmatchedRoute = "unmatched"

// HTTPCollector records router_api_requests_total and router_api_request_duration_seconds.
type HTTPCollector struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewHTTPCollector registers the API metrics on reg.
func NewHTTPCollector(reg prometheus.Registerer) *HTTPCollector {
	labels := []string{string(LabelRoute), string(LabelMethod), string(LabelStatus)}
	c := &HTTPCollector{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: string(RouterNamespace), Name: string(MetricAPIRequestsTotal), Help: "API requests by route, method and status.",
		}, labels),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: string(RouterNamespace), Name: string(MetricAPIRequestDuration), Help: "API request latency.",
			Buckets: prometheus.DefBuckets,
		}, labels[:2]),
	}
	reg.MustRegister(c.requests, c.duration)
	return c
}

// Observe records one finished request; an empty route is labelled unmatched.
func (c *HTTPCollector) Observe(route, method string, status int, elapsed time.Duration) {
	if route == "" {
		route = string(RouteUnmatched)
	}
	c.requests.WithLabelValues(route, method, strconv.Itoa(status)).Inc()
	c.duration.WithLabelValues(route, method).Observe(elapsed.Seconds())
}
