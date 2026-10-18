package validation

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// testCaseSeverity is how a failing saved test case counts per mode: it blocks deploys only; drafts and
// rollbacks (decision: rollback treats failing cases as warnings) list it as a warning.
var testCaseSeverity = map[ValidationMode]config.Severity{
	ModeDraft:    config.SeverityWarning,
	ModeDeploy:   config.SeverityError,
	ModeRollback: config.SeverityWarning,
}

// TestCasePointer is where test case issues point (the transform of the first branch, where cases are pinned).
var TestCasePointer = config.BranchPointer(0).Token(config.TokenTransform)

// TestCaseValidator runs the route's saved test cases (transform-studio) against the document.
type TestCaseValidator struct {
	Runner TestCaseRunner
}

func (TestCaseValidator) Name() ValidatorName { return ValidatorTestCase }

func (TestCaseValidator) Modes() []ValidationMode {
	return []ValidationMode{ModeDraft, ModeDeploy, ModeRollback}
}

// Validate reports every failing case; new routes (no id yet) have no saved cases.
func (v TestCaseValidator) Validate(ctx context.Context, in ValidationInput) ([]config.Issue, error) {
	if in.RouteID == "" || in.Config == nil {
		return nil, nil
	}
	results, err := v.Runner.Run(ctx, in.RouteID, in.Raw)
	if err != nil {
		return nil, err
	}
	severity := testCaseSeverity[in.Mode]
	issues := []config.Issue{}
	for _, r := range results {
		if r.Passed {
			continue
		}
		issues = append(issues, config.Issue{Code: config.IssueTestCaseFailed, Severity: severity, Path: TestCasePointer,
			Message: "Test case \"" + r.Name + "\" fails: " + r.Message})
	}
	return issues, nil
}
