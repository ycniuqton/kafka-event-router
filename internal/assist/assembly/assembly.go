// Package assembly composes the assistant (provider, proposer, explainer, exporter, HTTP module)
// from its settings; cmd/api only registers providers and mounts the result.
package assembly

import (
	"time"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/assist/describe"
	"github.com/eventrouter/router/internal/assist/explain"
	"github.com/eventrouter/router/internal/assist/export"
	assisthttp "github.com/eventrouter/router/internal/assist/http"
	"github.com/eventrouter/router/internal/assist/infer"
	"github.com/eventrouter/router/internal/platform/api"
)

// Deps are the inputs of the assistant assembly; Limiter (shared, e.g. Redis), Samples and Metrics are
// optional (in-memory window limiter, no samples, no metrics).
type Deps struct {
	Settings  assist.Settings
	Providers *assist.ProviderRegistry
	Responder *api.Responder
	Now       func() time.Time
	Limiter   assist.RequestLimiter
	Samples   assisthttp.SampleMasker
	Metrics   assist.Metrics
	Toggle    assisthttp.WorkspaceToggle
}

// Components are the built assistant services and its HTTP module.
type Components struct {
	Providers *assist.ProviderManager
	Proposer  *assist.StepProposer
	Explainer *explain.ExplainManager
	Exporter  *export.JsExporter
	Inferrer  *infer.MappingInferrer
	Module    api.Module
}

// Build composes the assistant; an unknown provider kind is an error.
func Build(d Deps) (*Components, error) {
	providers, err := d.Providers.Build(d.Settings)
	if err != nil {
		return nil, err
	}
	if d.Metrics == nil {
		d.Metrics = assist.NoMetrics{}
	}
	providers.WithMetrics(d.Metrics)
	checker, err := assist.NewStepChecker()
	if err != nil {
		return nil, err
	}
	matchers, err := infer.DefaultMatcherManager(d.Settings.InferMatchers)
	if err != nil {
		return nil, err
	}
	describer := describe.DefaultDescriberManager()
	c := &Components{
		Providers: providers,
		Proposer:  assist.NewStepProposer(providers, checker, describer),
		Explainer: explain.NewExplainManager(explain.NewRecipeExplainer(describer), explain.NewJsExplainer(providers)),
		Exporter:  export.NewJsExporter(export.DefaultJsWriterManager(), describer),
		Inferrer:  infer.NewMappingInferrer(matchers, infer.DefaultLimits),
	}
	c.Module = assisthttp.NewModule(assisthttp.Deps{
		Proposer: c.Proposer, Explainer: c.Explainer, Exporter: c.Exporter, Inferrer: c.Inferrer, Checker: checker,
		Samples: d.Samples, Metrics: d.Metrics, Responder: d.Responder, Limiter: Limiters{}.Pick(d),
		Toggle: d.Toggle,
	})
	return c, nil
}

// Limiters picks the request limiter: the shared one when wired, else a per-process window.
type Limiters struct{}

// Pick returns the limiter of d.
func (Limiters) Pick(d Deps) assist.RequestLimiter {
	if d.Limiter != nil {
		return d.Limiter
	}
	return assist.NewWindowLimiter(d.Settings.RatePerMinute, assist.MinuteWindow, d.Now)
}
