package assisthttp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/assist/infer"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/config"
)

// PathInferMapping keeps the system design's path; it is served by cmd/api (not the preview service).
const PathInferMapping Path = "/api/v1/preview/infer-mapping"

// SampleMasker masks samples before they reach a provider (live-preview's rule: sensitive connections
// and callers without raw access get masked values).
type SampleMasker interface {
	Mask(ctx context.Context, p auth.Principal, connectionID string, samples []json.RawMessage) []json.RawMessage
}

// DropSamples sends no sample values at all (used when no masking port is wired).
type DropSamples struct{}

// Mask implements SampleMasker.
func (DropSamples) Mask(context.Context, auth.Principal, string, []json.RawMessage) []json.RawMessage {
	return nil
}

// InferMappingHandler serves POST /api/v1/preview/infer-mapping {input, desiredOutput} (transform by example).
type InferMappingHandler struct {
	d    Deps
	gate RateGate
}

// Route implements api.Handler (produces config only, like a proposal: transform.write).
func (h *InferMappingHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathInferMapping), Permission: PermPropose, BodyLimit: api.BodyLimitLarge}
}

// Serve implements api.Handler; proposed steps are validated like assistant proposals (issues, never applied).
func (h *InferMappingHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	in, err := api.Decoder[infer.Request]{Limit: api.BodyLimitLarge}.Decode(w, r)
	if err != nil {
		return err
	}
	if err := h.gate.Check(w, r); err != nil {
		return err
	}
	start := time.Now()
	res, err := h.d.Inferrer.Infer(in)
	result := assist.ResultOK
	if err != nil {
		result = assist.ResultError
	}
	h.d.Metrics.Observe(assist.TaskInferMapping, assist.ProviderRules, result, time.Since(start))
	if err != nil {
		return err
	}
	issues, err := h.issues(res.Steps)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, InferResponse{Steps: res.Steps, Mappings: res.Mappings, Unmapped: res.Unmapped, Issues: issues})
}

// issues runs the route schema's step checks on the proposal (/steps/i paths).
func (h *InferMappingHandler) issues(raw []json.RawMessage) (config.Issues, error) {
	issues := config.Issues{}
	for i, r := range raw {
		var s config.Step
		if err := json.Unmarshal(r, &s); err != nil {
			return nil, err
		}
		issues = issues.Merge(h.d.Checker.Check(config.Pointer(assist.PathSteps).Index(i), s))
	}
	return issues, nil
}
