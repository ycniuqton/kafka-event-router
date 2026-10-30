package sandbox

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// CallOutcome describes one finished call for metrics; Kind is empty on success.
type CallOutcome struct {
	StepPath    string
	Kind        ErrorKind
	Duration    time.Duration
	CPUTime     time.Duration
	MemoryBytes int64
}

// Observer is the base of sandbox metrics sinks.
type Observer interface {
	CallObserved(scope RouteScope, out CallOutcome)
	QueueWaitObserved(scope RouteScope, wait time.Duration)
	PoolObserved(scope RouteScope, st PoolStats)
	ScopeClosed(scope RouteScope, steps []string)
}

// NopObserver discards observations (tests, tools).
type NopObserver struct{}

func (NopObserver) CallObserved(RouteScope, CallOutcome)        {}
func (NopObserver) QueueWaitObserved(RouteScope, time.Duration) {}
func (NopObserver) PoolObserved(RouteScope, PoolStats)          {}
func (NopObserver) ScopeClosed(RouteScope, []string)            {}

// MetricName names a sandbox metric.
type MetricName string

const (
	MetricTimeouts      MetricName = "router_js_timeouts_total"
	MetricMemoryBytes   MetricName = "router_js_memory_bytes"
	MetricCallDuration  MetricName = "router_js_call_duration_seconds"
	MetricErrors        MetricName = "router_js_errors_total"
	MetricPoolInstances MetricName = "router_js_pool_instances"
	MetricCPUSeconds    MetricName = "router_js_cpu_seconds_total"
	MetricQueueWait     MetricName = "router_js_queue_wait_seconds"
)

// MetricLabel names a metric label.
type MetricLabel string

const (
	LabelRouteID MetricLabel = "route_id"
	LabelVersion MetricLabel = "version"
	LabelKind    MetricLabel = "kind"
)

// DurationBuckets are the call-duration histogram buckets (seconds), centred on the 0.5 ms target.
var DurationBuckets = []float64{0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 1}

// QueueWaitBuckets are the queue-wait histogram buckets (seconds); most calls never wait.
var QueueWaitBuckets = []float64{0.00001, 0.0001, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.5}

// PromObserver exports sandbox metrics to Prometheus.
type PromObserver struct {
	timeouts  *prometheus.CounterVec
	memory    *prometheus.GaugeVec
	durations *prometheus.HistogramVec
	errors    *prometheus.CounterVec
	instances *prometheus.GaugeVec
	cpu       *prometheus.CounterVec
	queueWait *prometheus.HistogramVec
}

// HostOwnedSeries are the sandbox series the worker's route instruments already export with
// runner labels (monitoring); the worker excludes them so each series has one writer.
var HostOwnedSeries = []MetricName{MetricTimeouts, MetricMemoryBytes}

// NewPromObserver creates and registers the sandbox collectors on reg; excluded series are
// built but not registered, so their writes are dropped.
func NewPromObserver(reg prometheus.Registerer, excluded ...MetricName) (*PromObserver, error) {
	scope := []string{string(LabelRouteID), string(LabelVersion)}
	o := &PromObserver{
		timeouts: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricTimeouts),
			Help: "JS calls that hit the wall-clock limit."}, scope),
		memory: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: string(MetricMemoryBytes),
			Help: "Linear memory held by sandbox instances."}, scope),
		durations: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: string(MetricCallDuration),
			Help: "JS call duration.", Buckets: DurationBuckets}, scope),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricErrors),
			Help: "JS call errors by kind."}, append(scope, string(LabelKind))),
		instances: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: string(MetricPoolInstances),
			Help: "Live sandbox instances."}, scope),
		cpu: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricCPUSeconds),
			Help: "Guest execution time of JS calls (worker capacity scoring)."}, scope),
		queueWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: string(MetricQueueWait),
			Help: "Time a JS call waited for a free sandbox instance.", Buckets: QueueWaitBuckets}, scope),
	}
	if err := o.register(reg, excluded); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *PromObserver) register(reg prometheus.Registerer, excluded []MetricName) error {
	skip := map[MetricName]bool{}
	for _, name := range excluded {
		skip[name] = true
	}
	collectors := map[MetricName]prometheus.Collector{MetricTimeouts: o.timeouts, MetricMemoryBytes: o.memory,
		MetricCallDuration: o.durations, MetricErrors: o.errors, MetricPoolInstances: o.instances, MetricCPUSeconds: o.cpu, MetricQueueWait: o.queueWait}
	for name, c := range collectors {
		if skip[name] {
			continue
		}
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// CallObserved records duration and error counters of one call.
func (o *PromObserver) CallObserved(scope RouteScope, out CallOutcome) {
	route, version := scope.RouteID, scope.VersionLabel()
	o.durations.WithLabelValues(route, version).Observe(out.Duration.Seconds())
	o.cpu.WithLabelValues(route, version).Add(out.CPUTime.Seconds())
	if out.Kind == "" {
		return
	}
	o.errors.WithLabelValues(route, version, string(out.Kind)).Inc()
	if out.Kind == ErrorTimeout {
		o.timeouts.WithLabelValues(route, version).Inc()
	}
}

// QueueWaitObserved records how long one call waited for an instance.
func (o *PromObserver) QueueWaitObserved(scope RouteScope, wait time.Duration) {
	o.queueWait.WithLabelValues(scope.RouteID, scope.VersionLabel()).Observe(wait.Seconds())
}

// PoolObserved updates instance and memory gauges (summed over a scope's pools by the caller).
func (o *PromObserver) PoolObserved(scope RouteScope, st PoolStats) {
	o.instances.WithLabelValues(scope.RouteID, scope.VersionLabel()).Set(float64(st.Size))
	o.memory.WithLabelValues(scope.RouteID, scope.VersionLabel()).Set(float64(st.MemoryBytes))
}

// ScopeClosed zeroes the gauges of a closed route version.
func (o *PromObserver) ScopeClosed(scope RouteScope, _ []string) {
	o.instances.WithLabelValues(scope.RouteID, scope.VersionLabel()).Set(0)
	o.memory.WithLabelValues(scope.RouteID, scope.VersionLabel()).Set(0)
}
