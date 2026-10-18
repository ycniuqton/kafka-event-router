package connections

import (
	"context"
	"fmt"
	"time"

	"github.com/eventrouter/router/pkg/kafka"
)

// TestOptions are the optional topics of the read and write checks.
type TestOptions struct {
	ReadTopic  string
	WriteTopic string
}

// TestSession is what every check runs against: the spec, an open client and the options.
type TestSession struct {
	Spec    kafka.ConnectionSpec
	Client  kafka.Client
	Options TestOptions
}

// StepOutcome is the result of one check; Reason overrides classification when set.
type StepOutcome struct {
	State  StepState
	Err    error
	Reason kafka.FailureReason
}

// Passed, Skipped and Failed build outcomes.
func Passed() StepOutcome          { return StepOutcome{State: StatePassed} }
func Skipped() StepOutcome         { return StepOutcome{State: StateSkipped} }
func Failed(err error) StepOutcome { return StepOutcome{State: StateFailed, Err: err} }
func FailedAs(r kafka.FailureReason, err error) StepOutcome {
	return StepOutcome{State: StateFailed, Err: err, Reason: r}
}

// TestCheck is the base of every checklist step.
type TestCheck interface {
	Step() TestStep
	Run(ctx context.Context, s *TestSession) StepOutcome
}

// CheckRegistry maps steps to their check implementation.
type CheckRegistry struct {
	byStep map[TestStep]TestCheck
}

// NewCheckRegistry returns a registry holding checks.
func NewCheckRegistry(checks ...TestCheck) *CheckRegistry {
	r := &CheckRegistry{byStep: map[TestStep]TestCheck{}}
	for _, c := range checks {
		r.byStep[c.Step()] = c
	}
	return r
}

// For returns the check of a step.
func (r *CheckRegistry) For(step TestStep) (TestCheck, error) {
	c, ok := r.byStep[step]
	if !ok {
		return nil, fmt.Errorf("connections: no check registered for step %q", step)
	}
	return c, nil
}

// StepEventType distinguishes NDJSON lines.
type StepEventType string

const (
	EventStep   StepEventType = "step"
	EventResult StepEventType = "result"
)

// StepEvent is one streamed progress line.
type StepEvent struct {
	Type       StepEventType       `json:"type"`
	Step       TestStep            `json:"step"`
	State      StepState           `json:"state"`
	DurationMs *int64              `json:"durationMs,omitempty"`
	Reason     kafka.FailureReason `json:"reason,omitempty"`
	Message    string              `json:"message,omitempty"`
	Hint       string              `json:"hint,omitempty"`
}

// TestResult is the final line of a run.
type TestResult struct {
	Type        StepEventType `json:"type"`
	OK          bool          `json:"ok"`
	Health      HealthStatus  `json:"health"`
	CheckedAt   time.Time     `json:"checkedAt"`
	LastFailure *LastFailure  `json:"lastFailure,omitempty"`
}

// TestSink receives progress (implemented by the NDJSON writer and the prober collector).
type TestSink interface {
	Emit(e StepEvent)
	Finish(r TestResult)
}

// CollectingSink keeps every event and the result in memory.
type CollectingSink struct {
	Events []StepEvent
	Result *TestResult
}

// Emit implements TestSink.
func (s *CollectingSink) Emit(e StepEvent) { s.Events = append(s.Events, e) }

// Finish implements TestSink.
func (s *CollectingSink) Finish(r TestResult) { s.Result = &r }

// CheckRunner is the Manager executing a plan: per-step timeout, stop on first failure, skip the rest.
type CheckRunner struct {
	limits     RunnerLimits
	registry   *CheckRegistry
	classifier *kafka.FailureClassifier
	hints      *HintCatalog
	health     HealthMapper
	extensions PlanExtensions
	clock      Clock
}

// NewCheckRunner builds the runner.
func NewCheckRunner(registry *CheckRegistry, classifier *kafka.FailureClassifier, hints *HintCatalog, clock Clock) *CheckRunner {
	if clock == nil {
		clock = time.Now
	}
	limits := RunnerLimits{Step: time.Duration(StepTimeout), Run: time.Duration(RunTimeout)}
	return &CheckRunner{limits: limits, registry: registry, classifier: classifier, hints: hints, health: DefaultHealthMapper(),
		extensions: DefaultPlanExtensions, clock: clock}
}

// RunnerLimits are the per-step and per-run timeouts.
type RunnerLimits struct {
	Step time.Duration
	Run  time.Duration
}

// WithLimits returns a copy of the runner using other timeouts (tests, probes).
func (r *CheckRunner) WithLimits(l RunnerLimits) *CheckRunner {
	c := *r
	c.limits = l
	return &c
}

// Validate fails when a plan step has no registered check (startup guard).
func (r *CheckRunner) Validate(plan CheckPlan) error {
	for _, step := range plan {
		if _, err := r.registry.For(step); err != nil {
			return err
		}
	}
	return nil
}

// Run executes plan (plus applicable optional steps) against session, streaming to sink; it returns the result.
func (r *CheckRunner) Run(ctx context.Context, plan CheckPlan, s *TestSession, sink TestSink) TestResult {
	ctx, cancel := context.WithTimeout(ctx, r.limits.Run)
	defer cancel()
	var failure *LastFailure
	for _, step := range r.extensions.Apply(plan, s.Spec) {
		if failure != nil || ctx.Err() != nil {
			sink.Emit(StepEvent{Type: EventStep, Step: step, State: StateSkipped})
			continue
		}
		failure = r.runStep(ctx, step, s, sink)
	}
	result := TestResult{Type: EventResult, OK: failure == nil, Health: r.health.Map(failure), CheckedAt: r.clock().UTC(), LastFailure: failure}
	sink.Finish(result)
	return result
}

// runStep emits running, runs one check under the step timeout and emits its outcome.
func (r *CheckRunner) runStep(ctx context.Context, step TestStep, s *TestSession, sink TestSink) *LastFailure {
	sink.Emit(StepEvent{Type: EventStep, Step: step, State: StateRunning})
	start := r.clock()
	outcome := r.execute(ctx, step, s)
	elapsed := r.clock().Sub(start).Milliseconds()
	ev := StepEvent{Type: EventStep, Step: step, State: outcome.State, DurationMs: &elapsed}
	if outcome.State != StateFailed {
		sink.Emit(ev)
		return nil
	}
	ev.Reason = outcome.Reason
	if ev.Reason == "" {
		ev.Reason = r.classifier.Classify(outcome.Err)
	}
	hint := r.hints.Lookup(step, ev.Reason)
	ev.Message, ev.Hint = hint.Message, hint.Hint
	sink.Emit(ev)
	return &LastFailure{Step: step, Reason: ev.Reason, Message: hint.Message, Hint: hint.Hint}
}

func (r *CheckRunner) execute(ctx context.Context, step TestStep, s *TestSession) StepOutcome {
	check, err := r.registry.For(step)
	if err != nil {
		return Failed(err)
	}
	stepCtx, cancel := context.WithTimeout(ctx, r.limits.Step)
	defer cancel()
	outcome := check.Run(stepCtx, s)
	if outcome.State == StateFailed && outcome.Reason == "" && stepCtx.Err() != nil {
		outcome.Reason = kafka.ReasonTimeout
	}
	return outcome
}
