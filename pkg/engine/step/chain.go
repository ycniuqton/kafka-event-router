package step

import (
	"errors"
	"fmt"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// ErrTooManyOutputs is the cause of limit errors when a split exceeds maxOutputsPerEvent.
var ErrTooManyOutputs = errors.New("too many output events")

// ChainEntry is one enabled step with its document pointer.
type ChainEntry struct {
	Step    Step
	Pointer config.Pointer
}

// StepChain runs enabled steps in order (disabled steps are removed at compile).
type StepChain struct {
	entries    []ChainEntry
	maxOutputs int
}

// NewStepChain builds a chain bounded by maxOutputs events per input event.
func NewStepChain(entries []ChainEntry, maxOutputs int) *StepChain {
	return &StepChain{entries: entries, maxOutputs: maxOutputs}
}

// Entries returns the compiled steps in order.
func (c *StepChain) Entries() []ChainEntry { return c.entries }

// Len returns the number of steps.
func (c *StepChain) Len() int { return len(c.entries) }

// Run applies every step to every current event; an empty result means dropped.
func (c *StepChain) Run(sc *StepContext, tr trace.Tracer, ev engine.Event) ([]engine.Event, error) {
	current := []engine.Event{ev}
	for _, e := range c.entries {
		next, err := c.runStep(sc, tr, e, current)
		if err != nil {
			return nil, err
		}
		current = next
		if len(current) == 0 {
			break
		}
	}
	return current, nil
}

// runStep applies one step to all current events and traces the change.
func (c *StepChain) runStep(sc *StepContext, tr trace.Tracer, e ChainEntry, current []engine.Event) ([]engine.Event, error) {
	var before []any
	if tr.Enabled() {
		before = SnapshotList{}.Of(current)
	}
	start := time.Now()
	out, failure := c.applyAll(sc, e.Step, current)
	logs := sc.TakeLogs()
	if tr.Enabled() {
		tr.Step(trace.StepRecord{Branch: string(sc.Branch), StepID: e.Step.ID(), Kind: string(e.Step.Kind()),
			Before: before, After: SnapshotList{}.Of(out), Duration: time.Since(start), Logs: logs, Err: failure})
	}
	if failure != nil {
		return nil, ProcessErrors{}.ForStep(sc.Branch, e, failure)
	}
	return out, nil
}

func (c *StepChain) applyAll(sc *StepContext, s Step, current []engine.Event) ([]engine.Event, error) {
	out := make([]engine.Event, 0, len(current))
	for _, ev := range current {
		res, err := s.Apply(sc, ev)
		if err != nil {
			return out, err
		}
		out = append(out, res...)
	}
	if len(out) > c.maxOutputs {
		return out, fmt.Errorf("%w: %d > %d", ErrTooManyOutputs, len(out), c.maxOutputs)
	}
	return out, nil
}

// SnapshotList builds deep-copied JSON-shaped snapshots for tracing.
type SnapshotList struct{}

// Of snapshots every event.
func (SnapshotList) Of(evs []engine.Event) []any {
	out := make([]any, 0, len(evs))
	for _, ev := range evs {
		out = append(out, ev.Clone().Snapshot())
	}
	return out
}

// ProcessErrors maps step failures to typed engine errors.
type ProcessErrors struct{}

// ForStep classifies err: script timeout, fan-out limit or generic step error.
func (ProcessErrors) ForStep(branch engine.BranchID, e ChainEntry, err error) *engine.ProcessError {
	t := engine.ErrStep
	switch {
	case errors.Is(err, script.ErrTimeout):
		t = engine.ErrStepTimeout
	case errors.Is(err, ErrTooManyOutputs):
		t = engine.ErrLimit
	}
	return &engine.ProcessError{Type: t, Branch: branch, StepID: e.Step.ID(), Path: e.Pointer, Cause: err}
}
