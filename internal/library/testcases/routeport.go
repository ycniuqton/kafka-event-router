package testcases

import (
	"context"

	"github.com/eventrouter/router/internal/routes/validation"
)

// RoutePort adapts TestCaseService.RunForRoute to routes' validation.TestCaseRunner port: the routes
// test-case validator turns failing cases into deploy errors and draft/rollback warnings.
type RoutePort struct {
	Service *TestCaseService
}

// Run implements validation.TestCaseRunner; error verdicts count as failed.
func (p RoutePort) Run(ctx context.Context, routeID string, raw []byte) ([]validation.TestCaseResult, error) {
	report, err := p.Service.RunForRoute(ctx, routeID, raw)
	if err != nil {
		return nil, err
	}
	out := make([]validation.TestCaseResult, 0, len(report.Results))
	for _, r := range report.Results {
		out = append(out, validation.TestCaseResult{Name: r.Name, Passed: r.Result == ResultPass, Message: r.Error})
	}
	return out, nil
}

var _ validation.TestCaseRunner = RoutePort{}
