package dlq

import (
	"context"
	"errors"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine"
)

// DestinationProducer delivers encoded outputs to a destination connection.
type DestinationProducer interface {
	Produce(ctx context.Context, connectionID string, recs []engine.RawRecord) ([]Destination, error)
}

// InProcessExecutor runs the engine inside the API: decode, process, encode, produce.
// A retry that fails again is only reported; no new DLQ record is written.
type InProcessExecutor struct {
	Pipelines *PipelineCache
	Producer  DestinationProducer
}

// Kind implements RetryExecutor.
func (InProcessExecutor) Kind() ExecutorKind { return ExecutorInProcess }

// Execute implements RetryExecutor; a compile failure fails every record.
func (e InProcessExecutor) Execute(ctx context.Context, target RetryTarget, recs []record.Record) []ItemResult {
	compiled, err := e.Pipelines.Get(ctx, target.Route, target.Version)
	if err != nil {
		return ResultsOf{}.All(recs, ItemResult{Outcome: OutcomeFailed, ErrorType: FailCompile, Message: err.Error()})
	}
	run := &RetryRun{Compiled: compiled, Producer: e.Producer, Broken: map[string]string{}}
	out := make([]ItemResult, 0, len(recs))
	for _, rec := range recs {
		out = append(out, run.One(ctx, rec))
	}
	return out
}

// RetryRun is the state of one retry call: permanent destination failures stop later produces.
type RetryRun struct {
	Compiled *CompiledVersion
	Producer DestinationProducer
	Broken   map[string]string
}

// One retries a single record.
func (r *RetryRun) One(ctx context.Context, rec record.Record) ItemResult {
	p := r.Compiled.Pipeline
	ev, err := p.Decode(ctx, engine.RawRecord{Key: rec.Key, Value: rec.Value, Headers: rec.Headers,
		Topic: rec.Source.Topic, Partition: rec.Source.Partition, Offset: rec.Source.Offset, Timestamp: rec.Failure.FailedAt})
	if err != nil {
		return ProcessFailure{}.Result(rec.Ref, err)
	}
	outs, err := p.Process(ctx, ev, nil)
	if err != nil {
		return ProcessFailure{}.Result(rec.Ref, err)
	}
	outs = BranchScope{}.Keep(rec.Failure, outs)
	if len(outs) == 0 {
		return ItemResult{Ref: rec.Ref, Outcome: OutcomeFiltered}
	}
	byConn, err := r.encode(ctx, outs)
	if err != nil {
		return ProcessFailure{}.Result(rec.Ref, err)
	}
	return r.deliver(ctx, rec.Ref, byConn)
}

// encode encodes outputs grouped by destination connection.
func (r *RetryRun) encode(ctx context.Context, outs []engine.Out) (map[string][]engine.RawRecord, error) {
	byConn := map[string][]engine.RawRecord{}
	for _, o := range outs {
		raw, err := r.Compiled.Pipeline.Encode(ctx, o)
		if err != nil {
			return nil, err
		}
		conn := r.Compiled.Destinations[o.Branch]
		byConn[conn] = append(byConn[conn], raw)
	}
	return byConn, nil
}

// deliver produces to every destination; the first produced position is reported.
func (r *RetryRun) deliver(ctx context.Context, ref Ref, byConn map[string][]engine.RawRecord) ItemResult {
	var first *Destination
	for conn, raws := range byConn {
		if msg, broken := r.Broken[conn]; broken {
			return ItemResult{Ref: ref, Outcome: OutcomeFailed, ErrorType: FailDestination, Message: msg}
		}
		dests, err := r.Producer.Produce(ctx, conn, raws)
		if err != nil {
			if errors.Is(err, ErrPermanentDestination) {
				r.Broken[conn] = err.Error()
			}
			return ItemResult{Ref: ref, Outcome: OutcomeFailed, ErrorType: FailDestination, Message: err.Error()}
		}
		if first == nil && len(dests) > 0 {
			first = &dests[0]
		}
	}
	return ItemResult{Ref: ref, Outcome: OutcomeSent, Destination: first}
}

// BranchScope is branch-aware retry (FR-19): branches the worker delivered before the failure
// (router.delivered_branches) are not re-emitted, so healthy branches get no duplicates.
type BranchScope struct{}

// Keep returns the outputs of branches not yet delivered for the failed event.
func (BranchScope) Keep(f record.Failure, outs []engine.Out) []engine.Out {
	if f.Delivered == "" {
		return outs
	}
	kept := make([]engine.Out, 0, len(outs))
	for _, o := range outs {
		if !f.WasDelivered(string(o.Branch)) {
			kept = append(kept, o)
		}
	}
	return kept
}

// ProcessFailure converts engine errors to failed results.
type ProcessFailure struct{}

// Result returns a failed result with the mapped error type.
func (ProcessFailure) Result(ref Ref, err error) ItemResult {
	kind := FailStep
	if dep, ok := (DependencyKindOf{}).Of(err); ok {
		return ItemResult{Ref: ref, Outcome: OutcomeFailed, ErrorType: dep, Message: err.Error()}
	}
	var pe *engine.ProcessError
	if errors.As(err, &pe) {
		kind = FailureKindOf[record.FromEngine(pe.Type)]
	}
	return ItemResult{Ref: ref, Outcome: OutcomeFailed, ErrorType: kind, Message: err.Error()}
}

// ResultsOf builds result lists.
type ResultsOf struct{}

// All returns tmpl for every record.
func (ResultsOf) All(recs []record.Record, tmpl ItemResult) []ItemResult {
	out := make([]ItemResult, 0, len(recs))
	for _, r := range recs {
		tmpl.Ref = r.Ref
		out = append(out, tmpl)
	}
	return out
}
