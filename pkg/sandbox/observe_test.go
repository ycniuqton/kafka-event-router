package sandbox

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestPromObserverExcludesHostOwnedSeries(t *testing.T) {
	reg := prometheus.NewRegistry()
	host := prometheus.NewCounter(prometheus.CounterOpts{Name: string(MetricTimeouts), Help: "host-owned"})
	reg.MustRegister(host)
	if _, err := NewPromObserver(reg); err == nil {
		t.Fatal("duplicate registration must fail without exclusions")
	}
	reg = prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewCounter(prometheus.CounterOpts{Name: string(MetricTimeouts), Help: "host-owned"}))
	o, err := NewPromObserver(reg, HostOwnedSeries...)
	if err != nil {
		t.Fatal(err)
	}
	scope := RouteScope{RouteID: "r", VersionID: "1"}
	o.CallObserved(scope, CallOutcome{Kind: ErrorTimeout})
	o.PoolObserved(scope, PoolStats{Size: 2, MemoryBytes: 1})
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range families {
		names[f.GetName()] = true
	}
	for _, want := range []MetricName{MetricCallDuration, MetricErrors, MetricPoolInstances} {
		if !names[string(want)] {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	if names[string(MetricMemoryBytes)] {
		t.Fatal("excluded memory series must not be registered")
	}
}
