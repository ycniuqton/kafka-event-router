package runner

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// RecordProcessor runs decode → Pipeline.Process → encode per branch and binds outputs to destinations.
type RecordProcessor struct {
	pipeline     engine.Pipeline
	destinations map[engine.BranchID]config.Destination
}

// NewRecordProcessor indexes the branch destinations of v.
func NewRecordProcessor(v CompiledVersion) *RecordProcessor {
	dests := map[engine.BranchID]config.Destination{}
	for _, b := range v.Config.Branches {
		dests[engine.BranchID(b.ID)] = b.Destination
	}
	return &RecordProcessor{pipeline: v.Pipeline, destinations: dests}
}

// Process returns the encoded outputs of raw, or a typed engine error; panics become step errors.
// Pipelines implementing engine.RecordRunner run the whole record in one call (fast codec).
func (p *RecordProcessor) Process(ctx context.Context, raw engine.RawRecord) (outs []delivery.Outgoing, err error) {
	defer func() {
		if r := recover(); r != nil {
			outs, err = nil, &engine.ProcessError{Type: engine.ErrStep, Cause: &failures.PanicError{Value: r}}
		}
	}()
	if runner, ok := p.pipeline.(engine.RecordRunner); ok {
		return p.run(ctx, runner, raw)
	}
	ev, err := p.pipeline.Decode(ctx, raw)
	if err != nil {
		return nil, err
	}
	results, err := p.pipeline.Process(ctx, ev, trace.NoopTracer{})
	if err != nil {
		return nil, err
	}
	outs = make([]delivery.Outgoing, 0, len(results))
	for _, o := range results {
		out, err := p.encode(ctx, o)
		if err != nil {
			return nil, err
		}
		outs = append(outs, out)
	}
	return outs, nil
}

// run binds the records of a fused pipeline run to their destinations.
func (p *RecordProcessor) run(ctx context.Context, runner engine.RecordRunner, raw engine.RawRecord) ([]delivery.Outgoing, error) {
	encoded, err := runner.Run(ctx, raw)
	if err != nil {
		return nil, err
	}
	outs := make([]delivery.Outgoing, 0, len(encoded))
	for _, e := range encoded {
		outs = append(outs, p.bind(e.Record, e.Branch))
	}
	return outs, nil
}

func (p *RecordProcessor) encode(ctx context.Context, o engine.Out) (delivery.Outgoing, error) {
	rec, err := p.pipeline.Encode(ctx, o)
	if err != nil {
		return delivery.Outgoing{}, err
	}
	return p.bind(rec, o.Branch), nil
}

// bind stamps a missing timestamp and attaches the branch destination.
func (p *RecordProcessor) bind(rec engine.RawRecord, branch engine.BranchID) delivery.Outgoing {
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now()
	}
	dest := p.destinations[branch]
	mode := config.PartitioningModeByKey
	if dest.Partitioning != nil {
		mode = *dest.Partitioning
	}
	return delivery.Outgoing{Record: rec, ConnectionID: dest.ConnectionID, Partitioning: mode, Branch: branch}
}

// Close releases the pipeline.
func (p *RecordProcessor) Close(ctx context.Context) error {
	return p.pipeline.Close(ctx)
}
