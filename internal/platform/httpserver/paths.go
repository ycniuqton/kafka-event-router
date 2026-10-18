// Package httpserver provides the shared HTTP server with health probes and
// graceful shutdown used by every service.
package httpserver

// Path enumerates the well-known operational endpoints.
type Path string

const (
	PathHealthz Path = "/healthz"
	PathReadyz  Path = "/readyz"
	PathMetrics Path = "/metrics"
	PathAPIv1   Path = "/api/v1"
)

// ProbeStatus is the body reported by health probes.
type ProbeStatus string

const (
	ProbeOK       ProbeStatus = "ok"
	ProbeNotReady ProbeStatus = "not ready"
)

// Header names HTTP headers set by the server.
type Header string

const HeaderContentType Header = "Content-Type"

// MediaType names response content types.
type MediaType string

const MediaTypeJSON MediaType = "application/json"
