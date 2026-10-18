package compile

import (
	"context"
	"errors"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/filter"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/engine/step"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// branch is one compiled branch: optional filter, step chain, destination.
type branch struct {
	id         engine.BranchID
	filterBase config.Pointer
	filter     filter.Node
	chain      *step.StepChain
	output     *OutputShaper
}

// run evaluates the filter, then the chain; it returns tagged outputs and the branch status.
func (b *branch) run(r *eventRun, ev engine.Event) ([]engine.Out, trace.EventStatus, error) {
	pass, err := b.pass(r, &ev)
	if err != nil {
		return nil, trace.StatusError, err
	}
	if !pass {
		return nil, trace.StatusFiltered, nil
	}
	sc := &step.StepContext{Ctx: r.ctx, Now: r.now, Helpers: r.pipeline.helpers, Meta: r.pipeline.meta, Branch: b.id}
	evs, err := b.chain.Run(sc, r.tracer, ev)
	if err != nil {
		return nil, trace.StatusError, err
	}
	if len(evs) == 0 {
		return nil, trace.StatusDropped, nil
	}
	outs := make([]engine.Out, 0, len(evs))
	for _, e := range evs {
		outs = append(outs, engine.Out{Event: e, Branch: b.id})
	}
	return outs, trace.StatusPassed, nil
}

// pass evaluates the branch filter (nil = pass all) and types evaluation failures.
func (b *branch) pass(r *eventRun, ev *engine.Event) (bool, error) {
	if b.filter == nil {
		return true, nil
	}
	ec := &filter.EvalContext{Ctx: r.ctx, Event: ev, Now: r.now, Branch: b.id, Tracer: r.tracer}
	ok, err := b.filter.Eval(ec)
	if err == nil {
		return ok, nil
	}
	pe := &engine.ProcessError{Type: engine.ErrStep, Branch: b.id, StepID: engine.StepFilter, Path: b.filterBase, Cause: err}
	var fe *filter.FilterError
	if errors.As(err, &fe) {
		pe.Path = b.filterBase.Join(fe.Pointer)
	}
	if errors.Is(err, script.ErrTimeout) {
		pe.Type = engine.ErrStepTimeout
	}
	return false, pe
}

// Encode shapes key and headers, encodes with the destination codecs and enforces maxOutputBytes.
func (s *OutputShaper) Encode(ctx context.Context, o engine.Out, maxBytes int) (engine.RawRecord, error) {
	key, err := NullableEncoder{Codec: s.keyCodec}.Encode(ctx, s.key.Key(&o))
	if err != nil {
		return engine.RawRecord{}, s.invalid(o, err)
	}
	value, err := NullableEncoder{Codec: s.valCodec}.Encode(ctx, o.Value)
	if err != nil {
		return engine.RawRecord{}, s.invalid(o, err)
	}
	headers := append(s.headers.Select(o.Headers, s.keep), s.add...)
	rec := engine.RawRecord{Key: key, Value: value, Headers: headers, Topic: s.topic, Partition: o.Partition, Timestamp: o.Timestamp}
	if (RecordSize{}).Of(rec) > maxBytes {
		return engine.RawRecord{}, s.invalid(o, ErrOutputTooLarge)
	}
	return rec, nil
}

// invalid wraps an encode failure as output_invalid.
func (s *OutputShaper) invalid(o engine.Out, err error) error {
	return &engine.ProcessError{Type: engine.ErrOutputInvalid, Branch: o.Branch, Cause: err}
}

// RecordSize measures the produced bytes of a record.
type RecordSize struct{}

// Of returns key + value + header bytes.
func (RecordSize) Of(r engine.RawRecord) int {
	n := len(r.Key) + len(r.Value)
	for _, h := range r.Headers {
		n += len(h.Key) + len(h.Value)
	}
	return n
}

// NullableEncoder encodes nil as absent bytes (no key, tombstone value) regardless of format.
type NullableEncoder struct {
	Codec codec.Codec
}

// Encode returns nil bytes for nil and delegates otherwise.
func (e NullableEncoder) Encode(ctx context.Context, v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return e.Codec.Encode(ctx, v)
}
