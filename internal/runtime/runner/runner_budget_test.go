package runner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/clients"
	"github.com/eventrouter/router/internal/runtime/membudget"
	"github.com/eventrouter/router/internal/runtime/runner"
)

// budgetCase is a backlog far larger than a tiny worker budget, spread over a few keys; maxCost bounds the
// budget cost of one test record. Without the budget the lane caps alone let usage reach ~2 MiB here.
var budgetCase = struct {
	records int
	keys    int
	budget  int64
	maxCost int64
	timeout time.Duration
}{records: 6000, keys: 4, budget: 64 << 10, maxCost: 600, timeout: 30 * time.Second}

// produceSequenced writes records with key k<i%keys> and a per-key sequence number.
func produceSequenced(t *testing.T, c *kfake.Cluster) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.DefaultProduceTopic("in"))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	recs := make([]*kgo.Record, 0, budgetCase.records)
	for i := 0; i < budgetCase.records; i++ {
		key := i % budgetCase.keys
		val, _ := json.Marshal(map[string]any{"keep": true, "seq": i / budgetCase.keys})
		recs = append(recs, &kgo.Record{Key: []byte(fmt.Sprintf("k%d", key)), Value: val})
	}
	if err := cl.ProduceSync(context.Background(), recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}
}

// readOutput consumes every record of "out" and returns the sequence numbers per key in arrival order.
func readOutput(t *testing.T, c *kfake.Cluster) map[string][]int {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...), kgo.ConsumeTopics("out"), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	out, n := map[string][]int{}, 0
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for n < budgetCase.records && ctx.Err() == nil {
		cl.PollFetches(ctx).EachRecord(func(r *kgo.Record) {
			var v struct{ Seq int }
			_ = json.Unmarshal(r.Value, &v)
			out[string(r.Key)] = append(out[string(r.Key)], v.Seq)
			n++
		})
	}
	return out
}

// A backlog over the worker memory budget must pause fetching, resume under the low watermark and still
// deliver every record exactly once, in order per key.
func TestRunnerMemoryBudgetPausesResumesWithoutLossOrReorder(t *testing.T) {
	c := cluster(t)
	produceSequenced(t, c)
	budget := membudget.New(budgetCase.budget)
	deps := testDeps(clients.NewOpenerManager(clients.NewFranzOpener()), "w1", nil)
	deps.Budget = budget
	m := runner.NewManager(runner.RouteRunnerFactory{Deps: deps}, &haltRecorder{}, deps.Log)
	if err := m.Start(context.Background(), compiled(t, c, "dlq")); err != nil {
		t.Fatal(err)
	}
	peak := samplePeak(budget)
	began := time.Now()
	for countRecords(t, c, "out") < budgetCase.records {
		if time.Since(began) > budgetCase.timeout {
			t.Fatalf("delivered %d of %d records in %s", countRecords(t, c, "out"), budgetCase.records, budgetCase.timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
	top := peak()
	t.Logf("pauses=%d peak used=%d budget=%d", budget.Pauses(), top, budget.Limit())
	if budget.Pauses() < 2 {
		t.Fatalf("budget paused %d times, want repeated pause/resume cycles", budget.Pauses())
	}
	if bound := budget.Limit() + int64(runner.MaxPollRecords)*budgetCase.maxCost; top > bound {
		t.Fatalf("peak usage %d exceeds budget + one poll (%d): fetching was not paused", top, bound)
	}
	eventually(t, "budget released and resumed", func() bool { return budget.PausedRoutes() == 0 })
	m.StopAll(context.Background())
	if budget.Used() != 0 {
		t.Fatalf("budget still holds %d bytes after stop", budget.Used())
	}
	assertSequences(t, readOutput(t, c))
}

// samplePeak records the highest budget usage until the returned reader is called.
func samplePeak(b *membudget.Budget) func() int64 {
	var peak atomic.Int64
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
				if u := b.Used(); u > peak.Load() {
					peak.Store(u)
				}
			}
		}
	}()
	return func() int64 {
		select {
		case <-stop:
		default:
			close(stop)
		}
		return peak.Load()
	}
}

func assertSequences(t *testing.T, got map[string][]int) {
	t.Helper()
	per := budgetCase.records / budgetCase.keys
	for k := 0; k < budgetCase.keys; k++ {
		seqs := got[fmt.Sprintf("k%d", k)]
		if len(seqs) != per {
			t.Fatalf("key k%d: %d records, want %d (lost or duplicated)", k, len(seqs), per)
		}
		for i, s := range seqs {
			if s != i {
				t.Fatalf("key k%d: position %d holds seq %d (reordered)", k, i, s)
			}
		}
	}
}
