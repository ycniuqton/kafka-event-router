// Package metrics owns the Prometheus registry and /metrics handler for a
// service. Router metrics (section 12) register against Registry.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Namespace is the common prefix of all router metrics.
type Namespace string

const RouterNamespace Namespace = "router"

// Registry is a per-process Prometheus registry pre-loaded with runtime collectors.
type Registry struct {
	reg *prometheus.Registry
}

// NewRegistry returns a registry with Go runtime and process collectors.
func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return &Registry{reg: reg}
}

// Registerer exposes the registry for domain metrics.
func (r *Registry) Registerer() prometheus.Registerer {
	return r.reg
}

// Handler serves the registry in Prometheus exposition format.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{Registry: r.reg})
}
