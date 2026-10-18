package assisthttp

import (
	"encoding/json"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/assist/explain"
	"github.com/eventrouter/router/internal/assist/infer"
	"github.com/eventrouter/router/pkg/config"
)

// ProposeInput is the body of POST /assist/steps; route/branch/session ids are accepted for context.
// Samples (at most 2 reach the provider) are masked first when the connection's data is sensitive.
type ProposeInput struct {
	Instruction  string            `json:"instruction"`
	CurrentSteps []config.Step     `json:"currentSteps"`
	Fields       []string          `json:"fields"`
	Samples      []json.RawMessage `json:"samples,omitempty"`
	RouteID      string            `json:"routeId,omitempty"`
	BranchID     string            `json:"branchId,omitempty"`
	ConnectionID string            `json:"connectionId,omitempty"`
	SessionID    string            `json:"sessionId,omitempty"`
}

// InferResponse is the body returned by POST /preview/infer-mapping.
type InferResponse struct {
	Steps    []json.RawMessage `json:"steps"`
	Mappings []infer.Mapping   `json:"mappings"`
	Unmapped []string          `json:"unmapped"`
	Issues   config.Issues     `json:"issues"`
}

// ProposalDiff lists the proposal as diff lines (proposals only append steps today).
type ProposalDiff struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Changed []string `json:"changed"`
}

// ProposalResponse is the body returned by POST /assist/steps.
type ProposalResponse struct {
	ProposalID string        `json:"proposalId"`
	Steps      []config.Step `json:"steps"`
	Diff       ProposalDiff  `json:"diff"`
	Issues     config.Issues `json:"issues"`
	Provider   string        `json:"provider"`
}

// ProposalAudit is the audited shape of a proposal.
type ProposalAudit struct {
	InstructionLength int    `json:"instructionLength"`
	StepCount         int    `json:"stepCount"`
	Provider          string `json:"provider"`
}

// ExplainInput is the body of POST /assist/explain: one step or a chain.
type ExplainInput struct {
	Step  *config.Step  `json:"step,omitempty"`
	Steps []config.Step `json:"steps,omitempty"`
}

// Validated returns the steps to explain (a single step wins over the list).
func (in ExplainInput) Validated() ([]config.Step, error) {
	if in.Step != nil {
		return []config.Step{*in.Step}, nil
	}
	return in.Steps, StepCount{}.Check(in.Steps)
}

// ExplanationResponse is the body returned by POST /assist/explain.
type ExplanationResponse struct {
	Summary  string   `json:"summary"`
	Bullets  []string `json:"bullets"`
	Provider string   `json:"provider"`
}

// ExportInput is the body of POST /assist/export-js.
type ExportInput struct {
	Steps []config.Step `json:"steps"`
}

// ExportResponse is the body returned by POST /assist/export-js.
type ExportResponse struct {
	Step config.Step `json:"step"`
}

// StepCount validates the size of a steps list.
type StepCount struct{}

// Check requires 1..MaxSteps steps.
func (StepCount) Check(steps []config.Step) error {
	if len(steps) == 0 {
		return assist.ErrNoSteps
	}
	if len(steps) > assist.DefaultLimits().MaxSteps {
		return assist.ErrTooManySteps
	}
	return nil
}

// Presenter renders domain results as response bodies.
type Presenter struct{}

// Proposal renders a proposal.
func (Presenter) Proposal(p assist.Proposal) ProposalResponse {
	issues := p.Issues
	if issues == nil {
		issues = config.Issues{}
	}
	return ProposalResponse{ProposalID: p.ID, Steps: p.Steps, Issues: issues, Provider: string(p.Provider),
		Diff: ProposalDiff{Added: p.Added, Removed: []string{}, Changed: []string{}}}
}

// Explanation renders an explanation.
func (Presenter) Explanation(e explain.Explanation) ExplanationResponse {
	bullets := e.Bullets
	if bullets == nil {
		bullets = []string{}
	}
	return ExplanationResponse{Summary: e.Summary, Bullets: bullets, Provider: string(e.Source)}
}
