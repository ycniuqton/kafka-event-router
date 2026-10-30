package lookups

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/pkg/lookup"
)

// TestResult is the outcome of one Test button run.
type TestResult struct {
	OK        bool
	Health    HealthStatus
	CheckedAt time.Time
	Steps     []lookup.TestStepResult
}

// StageHealth maps the failed probe stage to the source health.
var StageHealth = map[lookup.ProbeStage]HealthStatus{
	lookup.StageReachable:     HealthUnreachable,
	lookup.StageAuthenticated: HealthAuthFailed,
	lookup.StageSample:        HealthDegraded,
}

// TestService runs the checklist on saved sources (health is stored) and unsaved drafts.
type TestService struct {
	Service *Service
	Tester  lookup.Tester
	Now     func() time.Time
}

// Saved tests a stored source with its decrypted secrets and records the health.
func (t TestService) Saved(ctx context.Context, id uuid.UUID) (TestResult, error) {
	src, spec, err := t.Service.Resolve(ctx, id)
	if err != nil {
		return TestResult{}, err
	}
	res := t.result(t.Tester.Run(ctx, spec))
	if err := t.Service.RecordHealth(ctx, id, t.health(res)); err != nil {
		return TestResult{}, err
	}
	return res, Auditor{}.Record(ctx, ActionTested, src, nil, TestAudit{OK: res.OK, Health: res.Health})
}

// Draft validates and tests an unsaved draft (nothing is stored).
func (t TestService) Draft(ctx context.Context, d Draft) (TestResult, error) {
	v := t.Service.d.Validator
	d = v.Prepare(d)
	if err := v.Validate(d); err != nil {
		return TestResult{}, err
	}
	res := t.result(t.Tester.Run(ctx, d.Spec()))
	src := Source{TeamID: d.TeamID}
	return res, Auditor{}.Record(ctx, ActionTested, src, nil, TestAudit{OK: res.OK, Health: res.Health, Draft: true})
}

func (t TestService) result(r lookup.TestReport) TestResult {
	now := time.Now
	if t.Now != nil {
		now = t.Now
	}
	health := HealthHealthy
	if !r.OK {
		health = StageHealth[r.FailedStage()]
	}
	return TestResult{OK: r.OK, Health: health, CheckedAt: now().UTC(), Steps: r.Steps}
}

func (TestService) health(r TestResult) Health {
	checked := r.CheckedAt
	h := Health{Status: r.Health, CheckedAt: &checked}
	for _, s := range r.Steps {
		if s.State == lookup.TestFailed {
			h.LastFailure = &LastFailure{Step: s.Stage, Message: s.Message, Hint: s.Hint}
		}
	}
	return h
}
