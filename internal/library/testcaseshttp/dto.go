package testcaseshttp

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/library/testcases"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// CreateInput is the body of POST /{routes|transforms}/{id}/test-cases.
type CreateInput struct {
	BranchID        string            `json:"branchId"`
	Name            string            `json:"name"`
	InputEvent      json.RawMessage   `json:"inputEvent"`
	ExpectedOutcome testcases.Outcome `json:"expectedOutcome"`
	ExpectedOutput  json.RawMessage   `json:"expectedOutput"`
}

// PatchInput is the body of PATCH /{routes|transforms}/{id}/test-cases/{caseId}; absent members stay unchanged.
type PatchInput struct {
	Name            *string            `json:"name"`
	ExpectedOutcome *testcases.Outcome `json:"expectedOutcome"`
	ExpectedOutput  json.RawMessage    `json:"expectedOutput"`
}

// RunInput is the body of POST /{routes|transforms}/{id}/test-cases/run.
type RunInput struct {
	Config  json.RawMessage `json:"config"`
	Steps   json.RawMessage `json:"steps"`
	CaseIDs []uuid.UUID     `json:"caseIds"`
}

// CaseDTO is one case in responses.
type CaseDTO struct {
	ID              uuid.UUID           `json:"id"`
	RouteID         uuid.UUID           `json:"routeId"`
	OwnerKind       testcases.OwnerKind `json:"ownerKind"`
	BranchID        string              `json:"branchId"`
	Name            string              `json:"name"`
	InputEvent      json.RawMessage     `json:"inputEvent"`
	ExpectedOutcome testcases.Outcome   `json:"expectedOutcome"`
	ExpectedOutput  json.RawMessage     `json:"expectedOutput"`
	CreatedBy       string              `json:"createdBy"`
	CreatedAt       time.Time           `json:"createdAt"`
	UpdatedAt       time.Time           `json:"updatedAt"`
	Version         int64               `json:"version"`
}

// ResultDTO is one verdict of a run.
type ResultDTO struct {
	CaseID        uuid.UUID            `json:"caseId"`
	Name          string               `json:"name"`
	BranchID      string               `json:"branchId"`
	Result        testcases.CaseResult `json:"result"`
	ActualOutcome testcases.Outcome    `json:"actualOutcome"`
	ActualOutput  json.RawMessage      `json:"actualOutput"`
	Diff          []trace.PatchOp      `json:"diff"`
	Error         string               `json:"error,omitempty"`
}

// ReportDTO is the response of a run.
type ReportDTO struct {
	Results []ResultDTO `json:"results"`
	Passed  int         `json:"passed"`
	Failed  int         `json:"failed"`
}

// ListDTO wraps listings.
type ListDTO struct {
	Items []CaseDTO `json:"items"`
}

// Presenter renders cases and reports.
type Presenter struct{}

// Case renders one case.
func (Presenter) Case(c testcases.TestCase) CaseDTO {
	return CaseDTO{ID: c.ID, RouteID: c.Owner.ID, OwnerKind: c.Owner.Kind, BranchID: c.BranchID, Name: c.Name, InputEvent: c.InputEvent,
		ExpectedOutcome: c.ExpectedOutcome, ExpectedOutput: c.ExpectedOutput, CreatedBy: c.CreatedBy,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Version: int64(c.RowVersion)}
}

// Report renders a run report.
func (Presenter) Report(rep testcases.RunReport) ReportDTO {
	out := ReportDTO{Results: make([]ResultDTO, 0, len(rep.Results)), Passed: rep.Passed, Failed: rep.Failed}
	for _, r := range rep.Results {
		out.Results = append(out.Results, ResultDTO{CaseID: r.CaseID, Name: r.Name, BranchID: r.BranchID, Result: r.Result,
			ActualOutcome: r.ActualOutcome, ActualOutput: r.ActualOutput, Diff: r.Diff, Error: r.Error})
	}
	return out
}
