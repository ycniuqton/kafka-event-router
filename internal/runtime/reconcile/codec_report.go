package reconcile

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/engine"
)

// Codec report metric and labels.
const (
	MetricRouteCodec runner.MetricName  = "router_route_codec_total"
	LabelCodec       runner.MetricLabel = "codec"
	LabelFallback    runner.MetricLabel = "fallback"
)

// CodecAttr names the log attributes of the codec report.
type CodecAttr string

const (
	AttrCodec    CodecAttr = "codec"
	AttrFallback CodecAttr = "fallback"
	AttrDetail   CodecAttr = "detail"
)

// CodecReport logs and counts the codec plan of every compiled version, so routes that
// fall back to the tree codec are visible with their reason.
type CodecReport struct {
	compiles *prometheus.CounterVec
}

// NewCodecReport registers router_route_codec_total (nil registerer = unregistered).
func NewCodecReport(reg prometheus.Registerer) *CodecReport {
	c := &CodecReport{compiles: prometheus.NewCounterVec(prometheus.CounterOpts{Name: string(MetricRouteCodec)},
		[]string{string(LabelCodec), string(LabelFallback)})}
	if reg != nil {
		reg.MustRegister(c.compiles)
	}
	return c
}

// Report records the plan of v when its pipeline exposes one.
func (c *CodecReport) Report(log *slog.Logger, a domain.Assignment, v runner.CompiledVersion) {
	r, ok := v.Pipeline.(engine.RecordRunner)
	if c == nil || !ok {
		return
	}
	plan := r.CodecPlan()
	c.compiles.WithLabelValues(string(plan.Mode), string(plan.Fallback)).Inc()
	if log != nil {
		log.Info("runtime: route codec", string(logging.AttrRouteID), a.RouteID, string(logging.AttrVersion), a.VersionNo,
			string(AttrCodec), string(plan.Mode), string(AttrFallback), string(plan.Fallback), string(AttrDetail), plan.Detail)
	}
}
