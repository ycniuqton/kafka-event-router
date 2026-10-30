package compile

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// ErrOutputTooLarge is the cause when an encoded record exceeds maxOutputBytes.
var ErrOutputTooLarge = errors.New("output record exceeds maxOutputBytes")

// ErrUnknownBranch is the cause when Encode receives an output of an unknown branch.
var ErrUnknownBranch = errors.New("unknown branch")

// compiledPipeline is the engine.Pipeline of one route version.
type compiledPipeline struct {
	meta      engine.RouteMeta
	helpers   ctxhelpers.Set
	modules   *script.ModuleSet
	limits    config.ResolvedLimits
	source    SourceCodecs
	branches  []*branch
	plan      engine.CodecPlan
	records   RecordStrategy
	fallbacks atomic.Uint64
}

// Decode decodes key and value once with the source codecs.
func (p *compiledPipeline) Decode(ctx context.Context, r engine.RawRecord) (engine.Event, error) {
	key, err := p.source.Key.Decode(ctx, r.Key)
	if err != nil {
		return engine.Event{}, DecodeErrors{}.Of(config.TokenKeyFormat, err)
	}
	value, err := p.source.Value.Decode(ctx, r.Value)
	if err != nil {
		return engine.Event{}, DecodeErrors{}.Of(config.TokenFormat, err)
	}
	return engine.Event{Key: key, Value: value, Headers: r.Headers, Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp}, nil
}

// Process runs every branch with the clock frozen for this event and finishes the trace.
func (p *compiledPipeline) Process(ctx context.Context, in engine.Event, tr trace.Tracer) ([]engine.Out, error) {
	tr = trace.OrNoop(tr)
	run := &eventRun{ctx: ctx, now: p.helpers.Clock.Now(), tracer: tr, pipeline: p}
	outs := []engine.Out{}
	statuses := StatusSet{}
	for i, b := range p.branches {
		res, status, err := b.run(run, BranchInput{}.For(in, i, len(p.branches)))
		if err != nil {
			tr.Finish(trace.StatusError, TraceErrors{}.Info(err))
			return nil, err
		}
		tr.Outputs(string(b.id), len(res))
		outs = append(outs, res...)
		statuses.Add(status)
	}
	if len(outs) > p.limits.MaxOutputsPerEvent {
		err := &engine.ProcessError{Type: engine.ErrLimit, Cause: fmt.Errorf("%d outputs > maxOutputsPerEvent %d", len(outs), p.limits.MaxOutputsPerEvent)}
		tr.Finish(trace.StatusError, TraceErrors{}.Info(err))
		return nil, err
	}
	tr.Finish(statuses.Final(), nil)
	return outs, nil
}

// Encode shapes and encodes one output for its branch destination.
func (p *compiledPipeline) Encode(ctx context.Context, o engine.Out) (engine.RawRecord, error) {
	for _, b := range p.branches {
		if b.id == o.Branch {
			return b.output.Encode(ctx, o, p.limits.MaxOutputBytes)
		}
	}
	return engine.RawRecord{}, &engine.ProcessError{Type: engine.ErrOutputInvalid, Branch: o.Branch, Cause: ErrUnknownBranch}
}

// Close releases sandbox modules.
func (p *compiledPipeline) Close(ctx context.Context) error {
	return p.modules.Close(ctx)
}

// eventRun is the per-event state shared by branches.
type eventRun struct {
	ctx      context.Context
	now      time.Time
	tracer   trace.Tracer
	pipeline *compiledPipeline
}

// DecodeErrors types decode failures; registry outages are decode_transient.
type DecodeErrors struct{}

// Of wraps err for the source member that failed.
func (DecodeErrors) Of(member config.PointerToken, err error) *engine.ProcessError {
	t := engine.ErrDecode
	if errors.Is(err, codec.ErrTransient) {
		t = engine.ErrDecodeTransient
	}
	return &engine.ProcessError{Type: t, Path: config.PointerRoot.Token(config.TokenSource).Token(member), Cause: err}
}

// BranchInput gives each branch an isolated event: every branch but the last works on a
// deep copy, the last one takes the original (copy-on-write: one copy fewer per event).
type BranchInput struct{}

// For returns the input of branch i out of n.
func (BranchInput) For(in engine.Event, i, n int) engine.Event {
	if i == n-1 {
		return in
	}
	return in.Clone()
}

// StatusSet aggregates branch outcomes into the event status.
type StatusSet struct {
	passed, dropped bool
}

// Add records one branch outcome.
func (s *StatusSet) Add(st trace.EventStatus) {
	s.passed = s.passed || st == trace.StatusPassed
	s.dropped = s.dropped || st == trace.StatusDropped
}

// Final returns passed if any branch produced output, else dropped, else filtered.
func (s StatusSet) Final() trace.EventStatus {
	switch {
	case s.passed:
		return trace.StatusPassed
	case s.dropped:
		return trace.StatusDropped
	}
	return trace.StatusFiltered
}

// TraceErrors converts processing errors to trace error info.
type TraceErrors struct{}

// Info describes err for the trace.
func (TraceErrors) Info(err error) *trace.ErrorInfo {
	var pe *engine.ProcessError
	if errors.As(err, &pe) {
		return &trace.ErrorInfo{Type: string(pe.Type), Step: pe.StepID, Path: string(pe.Path), Message: pe.Cause.Error()}
	}
	return &trace.ErrorInfo{Type: string(engine.ErrStep), Message: err.Error()}
}
