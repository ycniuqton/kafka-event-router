package assisthttp

import (
	"errors"
	"net/http"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/assist/explain"
	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/pkg/config"
)

// ProposeHandler serves POST /api/v1/assist/steps.
type ProposeHandler struct {
	d    Deps
	gate RateGate
}

// Route implements api.Handler.
func (h *ProposeHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathSteps), Permission: PermPropose, Mutating: true, AuditAction: ActionProposed,
		BodyLimit: api.BodyLimitLarge}
}

// Serve implements api.Handler.
func (h *ProposeHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	in, err := api.Decoder[ProposeInput]{Limit: api.BodyLimitLarge}.Decode(w, r)
	if err != nil {
		return err
	}
	samples := h.d.Samples.Mask(r.Context(), auth.PrincipalFrom(r.Context()), in.ConnectionID, in.Samples)
	req := assist.ProposalRequest{Instruction: in.Instruction, CurrentSteps: in.CurrentSteps, Fields: in.Fields, Samples: samples}
	if err := h.d.Proposer.Validate(req); err != nil {
		return err
	}
	if err := h.gate.Check(w, r); err != nil {
		return err
	}
	p, err := h.d.Proposer.Propose(r.Context(), req)
	if err != nil {
		return err
	}
	if err := h.audit(r, in, p); err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, Presenter{}.Proposal(p))
}

// audit records the proposal's shape only: instruction length and step count, never the text or samples.
func (h *ProposeHandler) audit(r *http.Request, in ProposeInput, p assist.Proposal) error {
	scope, ok := audit.FromContext(r.Context())
	if !ok {
		return nil
	}
	return scope.Record(r.Context(), audit.Change{Action: ActionProposed, Object: audit.ObjectRequest, ObjectID: p.ID,
		After: ProposalAudit{InstructionLength: len([]rune(in.Instruction)), StepCount: len(p.Steps), Provider: string(p.Provider)}})
}

// ExplainHandler serves POST /api/v1/assist/explain.
type ExplainHandler struct {
	d    Deps
	gate RateGate
}

// Route implements api.Handler.
func (h *ExplainHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathExplain), Permission: PermExplain, BodyLimit: api.BodyLimitLarge}
}

// Serve implements api.Handler.
func (h *ExplainHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	in, err := api.Decoder[ExplainInput]{Limit: api.BodyLimitLarge}.Decode(w, r)
	if err != nil {
		return err
	}
	steps, err := in.Validated()
	if err != nil {
		return err
	}
	if err := h.gate.Check(w, r); err != nil {
		return err
	}
	e, err := h.d.Explainer.Explain(r.Context(), steps)
	var unsupported explain.ErrKindUnsupported
	if errors.As(err, &unsupported) {
		return problem.NewError(problem.KindValidation, unsupported.Error()).At(string(assist.PathSteps)).Wrap(err)
	}
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, Presenter{}.Explanation(e))
}

// ExportJSHandler serves POST /api/v1/assist/export-js.
type ExportJSHandler struct {
	d Deps
}

// Route implements api.Handler.
func (h *ExportJSHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathExportJS), Permission: PermExport}
}

// Serve implements api.Handler.
func (h *ExportJSHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	in, err := api.Decoder[ExportInput]{}.Decode(w, r)
	if err != nil {
		return err
	}
	if err := (StepCount{}).Check(in.Steps); err != nil {
		return err
	}
	step, issues := h.d.Exporter.Export(config.Pointer(assist.PathSteps), in.Steps)
	if err := (IssueErrors{}).Of(issues); err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, ExportResponse{Step: step})
}
