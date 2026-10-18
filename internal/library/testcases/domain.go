// Package testcases holds saved test cases of a route branch (transform-studio P2): a pinned input
// event with its locked outcome and output, re-run in-process with pkg/engine against a draft.
package testcases

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// OwnerKind names what a test case belongs to (a route; library transforms come in P3).
type OwnerKind string

const (
	OwnerRoute     OwnerKind = "route"
	OwnerTransform OwnerKind = "transform"
)

// Owner identifies the object whose cases are listed or run.
type Owner struct {
	Kind OwnerKind
	ID   uuid.UUID
}

// Outcome is what processing did with an event (mirrors the preview trace statuses).
type Outcome string

const (
	OutcomePassed   Outcome = Outcome(trace.StatusPassed)
	OutcomeFiltered Outcome = Outcome(trace.StatusFiltered)
	OutcomeDropped  Outcome = Outcome(trace.StatusDropped)
	OutcomeError    Outcome = Outcome(trace.StatusError)
)

// KnownOutcomes lists the outcomes a case may lock.
var KnownOutcomes = map[Outcome]bool{OutcomePassed: true, OutcomeFiltered: true, OutcomeDropped: true, OutcomeError: true}

// CarriesOutput reports whether the outcome locks an output (only passed events have one).
func (o Outcome) CarriesOutput() bool { return o == OutcomePassed }

// CaseResult is the verdict of one case in a run.
type CaseResult string

const (
	ResultPass  CaseResult = "pass"
	ResultFail  CaseResult = "fail"
	ResultError CaseResult = "error"
)

// TestCase is one saved case.
type TestCase struct {
	ID              uuid.UUID
	Owner           Owner
	BranchID        string
	Name            string
	InputEvent      json.RawMessage
	ExpectedOutcome Outcome
	ExpectedOutput  json.RawMessage
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	RowVersion      store.RowVersion
}

// NewCase is a case to create.
type NewCase struct {
	BranchID        string
	Name            string
	InputEvent      json.RawMessage
	ExpectedOutcome Outcome
	ExpectedOutput  json.RawMessage
	Author          uuid.UUID
}

// CasePatch changes the name and/or the locked expectation; nil members stay unchanged.
type CasePatch struct {
	Name            *string
	ExpectedOutcome *Outcome
	ExpectedOutput  json.RawMessage
}

// RunRequest is a run of a route's cases against a draft: a full route document (config), or the
// stored draft with the first branch's steps replaced (steps). CaseIDs narrows the run.
type RunRequest struct {
	Config  json.RawMessage
	Steps   json.RawMessage
	CaseIDs []uuid.UUID
}

// Result is one case's verdict with the actual outcome and the RFC 6902 diff expected → actual.
type Result struct {
	CaseID        uuid.UUID
	Name          string
	BranchID      string
	Result        CaseResult
	ActualOutcome Outcome
	ActualOutput  json.RawMessage
	Diff          []trace.PatchOp
	Error         string
}

// RunReport is the outcome of a run.
type RunReport struct {
	Results []Result
	Passed  int
	Failed  int
}

// Add appends r and counts it (errors count as failed).
func (rep *RunReport) Add(r Result) {
	rep.Results = append(rep.Results, r)
	if r.Result == ResultPass {
		rep.Passed++
		return
	}
	rep.Failed++
}

// Limits caps cases per owner and the size of one input event (design §2.1).
type Limits struct {
	MaxCasesPerOwner  int
	MaxCaseEventBytes int
	MaxNameLength     int
}

// DefaultLimits are 200 cases per owner and 256 KB per input event.
var DefaultLimits = Limits{MaxCasesPerOwner: 200, MaxCaseEventBytes: 256 << 10, MaxNameLength: 200}

// Defaults of new cases.
type Default string

const (
	DefaultBranchID Default = "main"
	DefaultName     Default = "Test case"
)

// Field names the JSON Pointers of request members in validation errors.
type Field string

const (
	FieldName            Field = "/name"
	FieldBranchID        Field = "/branchId"
	FieldInputEvent      Field = "/inputEvent"
	FieldExpectedOutcome Field = "/expectedOutcome"
	FieldExpectedOutput  Field = "/expectedOutput"
	FieldConfig          Field = "/config"
	FieldSteps           Field = "/steps"
)

// Code names validation error codes.
type Code string

const (
	CodeRequired Code = "required"
	CodeInvalid  Code = "invalid"
	CodeTooLarge Code = "too_large"
	CodeTooMany  Code = "too_many"
)

// Message holds texts shown to users.
type Message string

const (
	MsgInputRequired   Message = "Pin an input event (an object with key, value and headers)."
	MsgInputTooLarge   Message = "The input event is larger than 256 KB."
	MsgTooManyCases    Message = "There are already 200 test cases here (the maximum). Delete one first."
	MsgOutcomeUnknown  Message = "Use one of passed, filtered, dropped or error."
	MsgOutputRequired  Message = "A passed outcome needs the expected output."
	MsgNameTooLong     Message = "Use a shorter name."
	MsgDraftRequired   Message = "Send the draft as config (route document) or steps."
	MsgDraftInvalid    Message = "The draft is not a valid route document."
	MsgNoDraft         Message = "The route has no saved configuration to run the steps in."
	MsgBranchMissing   Message = "The route has no branch with this id."
	MsgCompileFailed   Message = "The draft does not compile"
	MsgInputUndecoded  Message = "The input event no longer decodes"
	MsgOutcomeMismatch Message = "expected %s, got %s"
	MsgOutputMismatch  Message = "output differs at %d place(s)"
)

// ErrNotFound is returned for a missing case or route.
var ErrNotFound = store.ErrNotFound

// ErrUnknownComparator is returned when no comparator is registered for the configured kind.
var ErrUnknownComparator = errors.New("testcases: unknown comparator")

// ErrUnknownOwner is returned for an owner kind without a store column.
var ErrUnknownOwner = &store.Error{Kind: problem.KindNotFound, Detail: "Unknown test case owner."}
