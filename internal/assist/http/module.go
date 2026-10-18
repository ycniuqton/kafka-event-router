// Package assisthttp serves /api/v1/assist/* (step proposals, explanations, the JS export) and
// /api/v1/preview/infer-mapping (transform by example).
package assisthttp

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/assist/explain"
	"github.com/eventrouter/router/internal/assist/export"
	"github.com/eventrouter/router/internal/assist/infer"
	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/pkg/config"
)

// Path names the assist endpoints.
type Path string

const (
	PathSteps    Path = "/api/v1/assist/steps"
	PathExplain  Path = "/api/v1/assist/explain"
	PathExportJS Path = "/api/v1/assist/export-js"
)

// ActionProposed is the audit action of a step proposal (no instruction text or sample values are stored).
const ActionProposed audit.Action = "assist.proposed"

// RetryAfterHeader is the header telling rate-limited clients when to retry.
type RetryAfterHeader string

const HeaderRetryAfter RetryAfterHeader = "Retry-After"

// Permissions of the assist endpoints: proposing and exporting edit transforms, explaining only reads them.
var (
	PermPropose = rbac.PermTransformWrite.Auth()
	PermExplain = rbac.PermTransformRead.Auth()
	PermExport  = rbac.PermTransformWrite.Auth()
)

// Deps are the services the handlers call; Samples and Metrics default to no samples / no metrics.
type Deps struct {
	Proposer  *assist.StepProposer
	Explainer *explain.ExplainManager
	Exporter  *export.JsExporter
	Inferrer  *infer.MappingInferrer
	Checker   *assist.StepChecker
	Samples   SampleMasker
	Metrics   assist.Metrics
	Limiter   assist.RequestLimiter
	Toggle    WorkspaceToggle
	Responder *api.Responder
}

// WorkspaceToggle is the workspace "AI assist" setting (access-control's rbac.AIAssistGate); nil = always on.
type WorkspaceToggle interface {
	AIAssistEnabled(ctx context.Context) (bool, error)
}

// NewModule returns the assist handlers.
func NewModule(d Deps) api.Module {
	if d.Samples == nil {
		d.Samples = DropSamples{}
	}
	if d.Metrics == nil {
		d.Metrics = assist.NoMetrics{}
	}
	rate := RateGate{Limiter: d.Limiter}
	ai := RateGate{Limiter: d.Limiter, Toggle: d.Toggle}
	return api.Modules{&ProposeHandler{d: d, gate: ai}, &ExplainHandler{d: d, gate: ai}, &ExportJSHandler{d: d},
		&InferMappingHandler{d: d, gate: rate}}
}

// RateGate applies the workspace AI-assist setting (when Toggle is set) and the per-user assistant limit.
type RateGate struct {
	Limiter assist.RequestLimiter
	Toggle  WorkspaceToggle
}

// Check refuses calls while AI assist is off, then counts one call of the caller; refusals set Retry-After (seconds).
func (g RateGate) Check(w http.ResponseWriter, r *http.Request) error {
	if err := g.enabled(r.Context()); err != nil {
		return err
	}
	if g.Limiter == nil {
		return nil
	}
	ok, wait := g.Limiter.Allow(r.Context(), auth.PrincipalFrom(r.Context()).UserID.String())
	if ok {
		return nil
	}
	w.Header().Set(string(HeaderRetryAfter), strconv.Itoa(int(wait.Round(time.Second)/time.Second)+1))
	return assist.ErrRateLimited
}

// enabled reads the workspace toggle (nil toggle: on).
func (g RateGate) enabled(ctx context.Context) error {
	if g.Toggle == nil {
		return nil
	}
	on, err := g.Toggle.AIAssistEnabled(ctx)
	if err != nil {
		return err
	}
	if !on {
		return assist.ErrAssistOff
	}
	return nil
}

// IssueErrors turns config issues into a 422 validation error with JSON Pointer paths.
type IssueErrors struct{}

// Of returns nil when issues is empty.
func (IssueErrors) Of(issues config.Issues) error {
	verr := api.NewValidationError()
	for _, i := range issues {
		verr.Add(string(i.Path), string(i.Code), i.Message)
	}
	return verr.OrNil()
}
