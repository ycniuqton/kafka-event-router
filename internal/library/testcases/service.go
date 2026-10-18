package testcases

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/engine"
)

// ServiceDeps are the collaborators of the service; Targets defaults to routes only.
type ServiceDeps struct {
	Store   Store
	Routes  RouteSource
	Runner  *TestCaseRunner
	Limits  Limits
	Targets map[OwnerKind]TargetSource
}

// TestCaseService is the business entry point for saved test cases: CRUD per owner and runs.
type TestCaseService struct {
	d ServiceDeps
}

// NewService builds the service.
func NewService(d ServiceDeps) *TestCaseService {
	if d.Targets == nil {
		d.Targets = map[OwnerKind]TargetSource{OwnerRoute: RouteTargets{Routes: d.Routes}}
	}
	return &TestCaseService{d: d}
}

// Routes exposes the route source (owner-team lookup of the HTTP layer).
func (s *TestCaseService) Routes() RouteSource { return s.d.Routes }

// List returns the owner's cases.
func (s *TestCaseService) List(ctx context.Context, owner Owner) ([]TestCase, error) {
	return s.d.Store.List(ctx, owner)
}

// Get returns one case of the owner.
func (s *TestCaseService) Get(ctx context.Context, owner Owner, id uuid.UUID) (TestCase, error) {
	return s.d.Store.Get(ctx, owner, id)
}

// Create validates and stores a case; 422 past MaxCasesPerOwner.
func (s *TestCaseService) Create(ctx context.Context, owner Owner, c NewCase) (TestCase, error) {
	c = CaseValidator{Limits: s.d.Limits}.Defaults(c)
	if err := (CaseValidator{Limits: s.d.Limits}).New(c); err != nil {
		return TestCase{}, err
	}
	n, err := s.d.Store.Count(ctx, owner)
	if err != nil {
		return TestCase{}, err
	}
	if n >= s.d.Limits.MaxCasesPerOwner {
		return TestCase{}, api.NewValidationError().Add(string(FieldInputEvent), string(CodeTooMany), string(MsgTooManyCases))
	}
	return s.d.Store.Create(ctx, owner, c)
}

// Update renames a case or re-locks its expectation (If-Match row version).
func (s *TestCaseService) Update(ctx context.Context, owner Owner, id uuid.UUID, p CasePatch, expected store.RowVersion) (TestCase, error) {
	current, err := s.d.Store.Get(ctx, owner, id)
	if err != nil {
		return TestCase{}, err
	}
	p = PatchNormaliser{}.Normalise(p)
	if err := (CaseValidator{Limits: s.d.Limits}).Patch(p, current); err != nil {
		return TestCase{}, err
	}
	return s.d.Store.Update(ctx, owner, id, p, expected)
}

// Delete removes a case.
func (s *TestCaseService) Delete(ctx context.Context, owner Owner, id uuid.UUID) error {
	return s.d.Store.Delete(ctx, owner, id)
}

// Run executes the owner's cases (all, or req.CaseIDs) against the draft of req.
func (s *TestCaseService) Run(ctx context.Context, owner Owner, req RunRequest) (RunReport, error) {
	source, ok := s.d.Targets[owner.Kind]
	if !ok {
		return RunReport{}, ErrUnknownOwner
	}
	target, err := source.Target(ctx, owner.ID, req)
	if err != nil {
		return RunReport{}, err
	}
	cases, err := s.d.Store.List(ctx, owner)
	if err != nil {
		return RunReport{}, err
	}
	return s.d.Runner.Run(ctx, target, CaseSelection{IDs: req.CaseIDs}.Of(cases)), nil
}

// RunForRoute runs every case of the route against raw (the routes test-case validator's port).
func (s *TestCaseService) RunForRoute(ctx context.Context, routeID string, raw []byte) (RunReport, error) {
	id, err := uuid.Parse(routeID)
	if err != nil {
		return RunReport{Results: []Result{}}, nil
	}
	n, err := s.d.Store.Count(ctx, Owner{Kind: OwnerRoute, ID: id})
	if err != nil || n == 0 {
		return RunReport{Results: []Result{}}, err
	}
	return s.Run(ctx, Owner{Kind: OwnerRoute, ID: id}, RunRequest{Config: raw})
}

// CaseSelection narrows cases to the requested ids (all when none are given).
type CaseSelection struct {
	IDs []uuid.UUID
}

// Of returns the selected cases in stored order.
func (sel CaseSelection) Of(cases []TestCase) []TestCase {
	if len(sel.IDs) == 0 {
		return cases
	}
	want := map[uuid.UUID]bool{}
	for _, id := range sel.IDs {
		want[id] = true
	}
	out := []TestCase{}
	for _, c := range cases {
		if want[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

// PatchNormaliser clears the expected output when the outcome changes to one without output.
type PatchNormaliser struct{}

// Normalise returns p ready to store.
func (PatchNormaliser) Normalise(p CasePatch) CasePatch {
	if p.ExpectedOutcome != nil && !p.ExpectedOutcome.CarriesOutput() {
		p.ExpectedOutput = json.RawMessage(nullJSON)
	}
	return p
}

// CaseValidator checks new cases and patches against the limits (422 with JSON Pointer paths).
type CaseValidator struct {
	Limits Limits
}

// Defaults fills branch, name and outcome.
func (CaseValidator) Defaults(c NewCase) NewCase {
	if c.BranchID == "" {
		c.BranchID = string(DefaultBranchID)
	}
	if c.Name == "" {
		c.Name = string(DefaultName)
	}
	if c.ExpectedOutcome == "" {
		c.ExpectedOutcome = OutcomePassed
	}
	return c
}

// New validates a case to create.
func (v CaseValidator) New(c NewCase) error {
	errs := api.NewValidationError()
	v.input(errs, c.InputEvent)
	v.name(errs, c.Name)
	v.expectation(errs, c.ExpectedOutcome, c.ExpectedOutput)
	return errs.OrNil()
}

// Patch validates a patch against the stored case.
func (v CaseValidator) Patch(p CasePatch, current TestCase) error {
	errs := api.NewValidationError()
	if p.Name != nil {
		v.name(errs, *p.Name)
	}
	outcome, output := current.ExpectedOutcome, current.ExpectedOutput
	if p.ExpectedOutcome != nil {
		outcome = *p.ExpectedOutcome
	}
	if p.ExpectedOutput != nil {
		output = p.ExpectedOutput
	}
	v.expectation(errs, outcome, output)
	return errs.OrNil()
}

func (v CaseValidator) input(errs *api.ValidationError, raw json.RawMessage) {
	var ev engine.Event
	var probe map[string]json.RawMessage
	switch {
	case len(raw) > v.Limits.MaxCaseEventBytes:
		errs.Add(string(FieldInputEvent), string(CodeTooLarge), string(MsgInputTooLarge))
	case len(raw) == 0 || json.Unmarshal(raw, &probe) != nil || probe == nil || json.Unmarshal(raw, &ev) != nil:
		errs.Add(string(FieldInputEvent), string(CodeRequired), string(MsgInputRequired))
	}
}

func (v CaseValidator) name(errs *api.ValidationError, name string) {
	if len(name) > v.Limits.MaxNameLength {
		errs.Add(string(FieldName), string(CodeInvalid), string(MsgNameTooLong))
	}
}

func (CaseValidator) expectation(errs *api.ValidationError, outcome Outcome, output json.RawMessage) {
	if !KnownOutcomes[outcome] {
		errs.Add(string(FieldExpectedOutcome), string(CodeInvalid), string(MsgOutcomeUnknown))
		return
	}
	if outcome.CarriesOutput() && (len(output) == 0 || string(output) == nullJSON) {
		errs.Add(string(FieldExpectedOutput), string(CodeRequired), string(MsgOutputRequired))
	}
}
