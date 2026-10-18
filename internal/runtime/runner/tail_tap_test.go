package runner

import (
	"sync/atomic"
	"testing"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/engine"
)

// countingTap counts flag checks and offers; active switches the route's tail on.
type countingTap struct {
	active  bool
	checks  atomic.Int64
	offered atomic.Int64
}

func (c *countingTap) Active(string) bool {
	c.checks.Add(1)
	return c.active
}

func (c *countingTap) Offer(string, TailEvent) { c.offered.Add(1) }

func tailTicket() *recordTicket {
	st := &versionState{version: CompiledVersion{Ref: domain.VersionRef{RouteID: "r1"}}}
	return &recordTicket{state: st, raw: engine.RawRecord{Key: []byte("k"), Value: []byte(`{"a":1}`), Partition: 2, Offset: 7}}
}

func TestTailHookIsZeroCostWithoutActiveTail(t *testing.T) {
	tap := &countingTap{}
	hook, ticket := NewTailHook(tap), tailTicket()
	allocs := testing.AllocsPerRun(1000, func() { hook.Observe(ticket, domain.OutcomeProduced) })
	if allocs != 0 {
		t.Fatalf("inactive tail allocates %.1f per record", allocs)
	}
	if tap.offered.Load() != 0 || tap.checks.Load() == 0 {
		t.Fatalf("inactive tail: offered %d, checks %d", tap.offered.Load(), tap.checks.Load())
	}
	noop := testing.AllocsPerRun(1000, func() { NewTailHook(nil).Observe(ticket, domain.OutcomeFiltered) })
	if noop != 0 {
		t.Fatalf("NoopTail allocates %.1f per record", noop)
	}
}

func TestTailHookOffersRecordWhenTailed(t *testing.T) {
	tap := &countingTap{active: true}
	NewTailHook(tap).Observe(tailTicket(), domain.OutcomeFiltered)
	if tap.offered.Load() != 1 {
		t.Fatalf("tailed route: offered %d, want 1", tap.offered.Load())
	}
}

func BenchmarkTailHookInactive(b *testing.B) {
	hook, ticket := NewTailHook(&countingTap{}), tailTicket()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		hook.Observe(ticket, domain.OutcomeProduced)
	}
}
