// Live-tail call site (RR-205): a tailed route offers every finished record with input, outputs and outcome.
package runner_test

import (
	"context"
	"sync"
	"testing"

	"github.com/eventrouter/router/internal/runtime/clients"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
)

type tailRecorder struct {
	mu     sync.Mutex
	events []runner.TailEvent
}

func (r *tailRecorder) Active(routeID string) bool { return routeID == "r1" }

func (r *tailRecorder) Offer(_ string, ev runner.TailEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *tailRecorder) byOutcome() map[domain.RecordOutcome][]runner.TailEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[domain.RecordOutcome][]runner.TailEvent{}
	for _, ev := range r.events {
		out[ev.Outcome] = append(out[ev.Outcome], ev)
	}
	return out
}

func TestRunnerOffersFinishedRecordsToTheTail(t *testing.T) {
	c := cluster(t)
	tail := &tailRecorder{}
	deps := testDeps(clients.NewOpenerManager(clients.NewFranzOpener()), "w1", nil)
	deps.Tail = tail
	m := runner.NewManager(runner.RouteRunnerFactory{Deps: deps}, &haltRecorder{}, deps.Log)
	produce(t, c, `{"keep":true,"n":1}`, `{"keep":false}`, `not-json`)
	if err := m.Start(context.Background(), compiled(t, c, "dlq")); err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	eventually(t, "three tailed records", func() bool { return len(tail.byOutcome()) == 3 })
	got := tail.byOutcome()
	sent := got[domain.OutcomeProduced][0]
	if len(sent.Outputs) != 1 || string(sent.Input.Value) != `{"keep":true,"n":1}` || sent.Branch != "main" {
		t.Fatalf("produced tail event = %+v", sent)
	}
	if dl := got[domain.OutcomeDeadLettered][0]; dl.ErrorType != string(domain.CategoryDecode) || dl.Error == "" {
		t.Fatalf("dead-lettered tail event = %+v", dl)
	}
	if len(got[domain.OutcomeFiltered][0].Outputs) != 0 {
		t.Fatal("filtered record must have no outputs")
	}
}
