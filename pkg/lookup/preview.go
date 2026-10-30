package lookup

import (
	"context"
	"errors"
	"time"

	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// PreviewTiming bounds live lookups made by preview runs.
type PreviewTiming time.Duration

const PreviewCallCap PreviewTiming = PreviewTiming(2 * time.Second)

// MockAnswer is a typed answer the Studio user enters for one lookup step (`lookupMocks[stepId]`).
type MockAnswer struct {
	Found bool `json:"found"`
	Value any  `json:"value,omitempty"`
}

// PreviewGateway answers from per-step mocks, else calls the live gateway with a capped timeout.
type PreviewGateway struct {
	Live  ctxhelpers.SourceGateway
	Mocks map[string]MockAnswer
	Cap   time.Duration
}

// NewPreviewGateway builds the gateway of one preview run (nil live = mocks only, calls fail permanently).
func NewPreviewGateway(live ctxhelpers.SourceGateway, mocks map[string]MockAnswer) *PreviewGateway {
	if live == nil {
		live = ctxhelpers.NoSourceGateway{}
	}
	return &PreviewGateway{Live: live, Mocks: mocks, Cap: time.Duration(PreviewCallCap)}
}

// Fetch implements ctxhelpers.SourceGateway.
func (g *PreviewGateway) Fetch(ctx context.Context, call ctxhelpers.SourceCall) (ctxhelpers.SourceAnswer, error) {
	if m, ok := g.Mocks[call.StepID]; ok {
		return ctxhelpers.SourceAnswer{Found: m.Found, Value: codec.NumberPolicy{}.Normalized(m.Value), Mode: ctxhelpers.AnswerMock}, nil
	}
	capped, cancel := context.WithTimeout(ctx, g.Cap)
	defer cancel()
	answer, err := g.Live.Fetch(capped, call)
	answer.Mode = ctxhelpers.AnswerLive
	return answer, err
}

// Observe implements ctxhelpers.SourceGateway.
func (g *PreviewGateway) Observe(sourceID string, ev ctxhelpers.SourceEvent) {
	g.Live.Observe(sourceID, ev)
}

// TestState is the state of one checklist step.
type TestState string

const (
	TestPassed  TestState = "passed"
	TestFailed  TestState = "failed"
	TestSkipped TestState = "skipped"
)

// TestStepResult is one checklist row.
type TestStepResult struct {
	Stage    ProbeStage
	State    TestState
	Duration time.Duration
	Message  string
	Hint     string
}

// TestReport is the checklist of one test run.
type TestReport struct {
	OK    bool
	Steps []TestStepResult
}

// FailedStage returns the failed stage ("" when OK).
func (r TestReport) FailedStage() ProbeStage {
	for _, s := range r.Steps {
		if s.State == TestFailed {
			return s.Stage
		}
	}
	return ""
}

// TestTiming bounds one test run.
type TestTiming time.Duration

const DefaultTestTimeout TestTiming = TestTiming(10 * time.Second)

// Tester runs the reachable → authenticated → sample checklist of a source (Test button).
type Tester struct {
	Factories *FactoryRegistry
	Timeout   time.Duration
}

// Run builds a fresh client for spec, probes it and closes it.
func (t Tester) Run(ctx context.Context, spec SourceSpec) TestReport {
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = time.Duration(DefaultTestTimeout)
	}
	start := time.Now()
	client, err := t.Factories.New(spec)
	if err != nil {
		return TestReports{}.Of(&ProbeError{Stage: StageReachable, Hint: string(HintReachable), Cause: err}, time.Since(start))
	}
	defer client.Close()
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return TestReports{}.Of(client.Probe(probeCtx), time.Since(start))
}

// TestReports turns a probe outcome into the checklist.
type TestReports struct{}

// Of marks stages before the failed one passed, the failed one failed, later ones skipped.
func (TestReports) Of(err error, d time.Duration) TestReport {
	var pe *ProbeError
	if err != nil && !errors.As(err, &pe) {
		pe = &ProbeError{Stage: StageReachable, Hint: string(HintReachable), Cause: err}
	}
	report := TestReport{OK: pe == nil}
	state := TestPassed
	for _, stage := range ProbeStages {
		row := TestStepResult{Stage: stage, State: state}
		if pe != nil && stage == pe.Stage {
			row = TestStepResult{Stage: stage, State: TestFailed, Duration: d, Message: pe.Cause.Error(), Hint: pe.Hint}
			state = TestSkipped
		}
		report.Steps = append(report.Steps, row)
	}
	return report
}
