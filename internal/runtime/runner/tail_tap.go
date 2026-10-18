package runner

import (
	"time"

	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/engine"
)

// TailTap is the live-tail port of the runner (monitoring's Tap behind an adapter in cmd/worker).
// Active must be a cheap cached flag check: nothing is built for a route nobody tails.
type TailTap interface {
	Active(routeID string) bool
	Offer(routeID string, ev TailEvent)
}

// TailEvent is one finished record as the live tail shows it: input, outputs per branch and outcome.
type TailEvent struct {
	At        time.Time
	Partition int32
	Offset    int64
	Branch    engine.BranchID
	Outcome   domain.RecordOutcome
	Step      string
	ErrorType string
	Error     string
	Input     engine.RawRecord
	Outputs   []delivery.Outgoing
}

// NoopTail is the tap used when no live tail is wired (workers without Redis, tests).
type NoopTail struct{}

// Active implements TailTap.
func (NoopTail) Active(string) bool { return false }

// Offer implements TailTap.
func (NoopTail) Offer(string, TailEvent) {}

// TailHook is the runner call site of the live tail, run after each record outcome.
type TailHook struct {
	tap TailTap
}

// NewTailHook wraps tap (nil = NoopTail).
func NewTailHook(tap TailTap) TailHook {
	if tap == nil {
		tap = NoopTail{}
	}
	return TailHook{tap: tap}
}

// Observe offers the finished record when its route is tailed; otherwise it costs one flag check.
func (h TailHook) Observe(t *recordTicket, outcome domain.RecordOutcome) {
	routeID := t.state.version.Ref.RouteID
	if !h.tap.Active(routeID) {
		return
	}
	h.tap.Offer(routeID, t.tailEvent(outcome))
}

// tailEvent describes the ticket's record for the live tail.
func (t *recordTicket) tailEvent(outcome domain.RecordOutcome) TailEvent {
	ev := TailEvent{At: time.Now(), Partition: t.raw.Partition, Offset: t.raw.Offset, Outcome: outcome, Input: t.raw, Outputs: t.outs}
	if len(t.outs) > 0 {
		ev.Branch = t.outs[0].Branch
	}
	if f := t.failure; f != nil {
		ev.Step, ev.ErrorType, ev.Error = f.Step(), string(f.Category), f.Message()
		ev.Branch = engine.BranchID(f.Branch())
	}
	return ev
}
