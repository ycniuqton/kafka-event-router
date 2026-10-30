package membudget

import (
	"github.com/prometheus/client_golang/prometheus"
)

// MetricName names the budget gauges.
type MetricName string

const (
	MetricBudgetBytes  MetricName = "router_worker_memory_budget_bytes"
	MetricUsedBytes    MetricName = "router_worker_memory_used_bytes"
	MetricPausedRoutes MetricName = "router_worker_memory_paused_routes"
	MetricPauses       MetricName = "router_worker_memory_pauses_total"
)

// MetricLabel names the budget gauge labels.
type MetricLabel string

const LabelWorker MetricLabel = "worker"

// Register exposes the budget as gauges read at scrape time (nothing on the record path).
func (b *Budget) Register(reg prometheus.Registerer, worker string) error {
	labels := prometheus.Labels{string(LabelWorker): worker}
	gauges := map[MetricName]func() float64{
		MetricBudgetBytes:  func() float64 { return float64(b.Limit()) },
		MetricUsedBytes:    func() float64 { return float64(b.Used()) },
		MetricPausedRoutes: func() float64 { return float64(b.PausedRoutes()) },
	}
	for name, read := range gauges {
		g := prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: string(name), ConstLabels: labels}, read)
		if err := reg.Register(g); err != nil {
			return err
		}
	}
	return reg.Register(prometheus.NewCounterFunc(prometheus.CounterOpts{Name: string(MetricPauses), ConstLabels: labels},
		func() float64 { return float64(b.Pauses()) }))
}
