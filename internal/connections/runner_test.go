package connections_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/pkg/kafka"
)

// fakeCheck returns a fixed outcome, or blocks until its context ends.
type fakeCheck struct {
	step    connections.TestStep
	outcome connections.StepOutcome
	block   bool
}

func (f fakeCheck) Step() connections.TestStep { return f.step }

func (f fakeCheck) Run(ctx context.Context, _ *connections.TestSession) connections.StepOutcome {
	if f.block {
		<-ctx.Done()
		return connections.Failed(ctx.Err())
	}
	return f.outcome
}

func runner(checks ...connections.TestCheck) *connections.CheckRunner {
	r := connections.NewCheckRunner(connections.NewCheckRegistry(checks...), kafka.DefaultFailureClassifier(), connections.DefaultHintCatalog(), nil)
	return r.WithLimits(connections.RunnerLimits{Step: 50 * time.Millisecond, Run: time.Second})
}

func states(events []connections.StepEvent) map[connections.TestStep]connections.StepState {
	out := map[connections.TestStep]connections.StepState{}
	for _, e := range events {
		out[e.Step] = e.State
	}
	return out
}

func TestCheckRunnerStopsOnFailure(t *testing.T) {
	r := runner(
		fakeCheck{step: connections.StepBrokersReachable, outcome: connections.Passed()},
		fakeCheck{step: connections.StepAuthenticated, outcome: connections.FailedAs(kafka.ReasonSASLMismatch, errors.New("x"))},
		fakeCheck{step: connections.StepListTopics, outcome: connections.Passed()},
	)
	sink := &connections.CollectingSink{}
	plan := connections.CheckPlan{connections.StepBrokersReachable, connections.StepAuthenticated, connections.StepListTopics}
	res := r.Run(context.Background(), plan, &connections.TestSession{}, sink)
	if res.OK || res.Health != connections.HealthAuthFailed || res.LastFailure.Reason != kafka.ReasonSASLMismatch {
		t.Fatalf("result: %+v", res)
	}
	st := states(sink.Events)
	if st[connections.StepBrokersReachable] != connections.StatePassed || st[connections.StepListTopics] != connections.StateSkipped {
		t.Fatalf("states: %v", st)
	}
	if sink.Events[0].State != connections.StateRunning || sink.Result == nil {
		t.Fatal("first event must be running and the result must be finished")
	}
	failed := sink.Events[3]
	if failed.Message == "" || failed.Hint == "" || failed.DurationMs == nil {
		t.Fatalf("failed event: %+v", failed)
	}
}

func TestCheckRunnerTimeoutAndSkips(t *testing.T) {
	r := runner(
		fakeCheck{step: connections.StepBrokersReachable, block: true},
		fakeCheck{step: connections.StepCanWrite, outcome: connections.Skipped()},
	)
	sink := &connections.CollectingSink{}
	res := r.Run(context.Background(), connections.CheckPlan{connections.StepBrokersReachable}, &connections.TestSession{}, sink)
	if res.Health != connections.HealthUnreachable || res.LastFailure.Reason != kafka.ReasonTimeout {
		t.Fatalf("timeout: %+v", res)
	}
	res = r.Run(context.Background(), connections.CheckPlan{connections.StepCanWrite}, &connections.TestSession{}, &connections.CollectingSink{})
	if !res.OK || res.Health != connections.HealthHealthy {
		t.Fatalf("skipped write must stay healthy: %+v", res)
	}
}

func TestCheckRunnerCancelled(t *testing.T) {
	r := runner(fakeCheck{step: connections.StepBrokersReachable, outcome: connections.Passed()}, fakeCheck{step: connections.StepAuthenticated, outcome: connections.Passed()})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sink := &connections.CollectingSink{}
	r.Run(ctx, connections.CheckPlan{connections.StepBrokersReachable, connections.StepAuthenticated}, &connections.TestSession{}, sink)
	for _, e := range sink.Events {
		if e.State != connections.StateSkipped {
			t.Fatalf("cancelled run must skip: %+v", e)
		}
	}
	if err := r.Validate(connections.CheckPlan{connections.StepSchemaRegistry}); err == nil {
		t.Fatal("unregistered step must fail validation")
	}
}
