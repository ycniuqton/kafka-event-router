package escapetest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/boundary"
)

// Outcome is the verdict of one case.
type Outcome struct {
	Case Case
	Err  error
}

// Runner executes cases against a Manager configured with production limits.
type Runner struct {
	Manager sandbox.Manager
	Scope   sandbox.RouteScope
}

// Run executes every case and returns one outcome per case.
func (r Runner) Run(ctx context.Context, cases []Case) []Outcome {
	out := make([]Outcome, 0, len(cases))
	for _, c := range cases {
		out = append(out, Outcome{Case: c, Err: r.runCase(ctx, c)})
	}
	return out
}

func (r Runner) runCase(ctx context.Context, c Case) error {
	src := sandbox.Source{StepPath: "/escape", Entry: sandbox.EntryTransform, Code: c.Code}
	rs, err := r.Manager.Load(ctx, r.Scope, []sandbox.Source{src}, c.Limits)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}
	defer rs.Close(ctx)
	mod, _ := rs.Module(src.StepPath)
	res, err := mod.Call(ctx, Event{}.At(attackOffset))
	if verr := c.Attack.Verify(res, err); verr != nil {
		return fmt.Errorf("attack: %w", verr)
	}
	res, err = mod.Call(ctx, Event{}.At(followUpOffset))
	if verr := c.After.Verify(res, err); verr != nil {
		return fmt.Errorf("next event: %w", verr)
	}
	return nil
}

// probeOffset is the event offset of each phase of a case.
type probeOffset = int64

const (
	attackOffset   probeOffset = 1
	followUpOffset probeOffset = 2
)

// Event builds the probe invocations.
type Event struct{}

// At returns an invocation for offset.
func (Event) At(offset int64) sandbox.Invocation {
	return sandbox.Invocation{Event: boundary.GuestEvent{Value: map[string]any{"probe": true}, Topic: "escape", Offset: offset},
		Now: time.Unix(1700000000, 0)}
}

// sprintf fills %s placeholders without fmt verbs leaking into JS.
func sprintf(format string, args ...string) string {
	for _, a := range args {
		format = strings.Replace(format, "%s", a, 1)
	}
	return format
}
