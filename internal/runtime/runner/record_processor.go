package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/eventrouter/router/internal/runtime/delivery"
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
func (p *RecordProcessor) Process(ctx context.Context, raw engine.RawRecord) (outs []delivery.Outgoing, err error) {
	defer func() {
		if r := recover(); r != nil {
			outs, err = nil, &engine.ProcessError{Type: engine.ErrStep, Cause: fmt.Errorf("panic: %v", r)}
		}
	}()
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

func (p *RecordProcessor) encode(ctx context.Context, o engine.Out) (delivery.Outgoing, error) {
	rec, err := p.pipeline.Encode(ctx, o)
	if err != nil {
		return delivery.Outgoing{}, err
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now()
	}
	dest := p.destinations[o.Branch]
	mode := config.PartitioningModeByKey
	if dest.Partitioning != nil {
		mode = *dest.Partitioning
	}
	return delivery.Outgoing{Record: rec, ConnectionID: dest.ConnectionID, Partitioning: mode, Branch: o.Branch}, nil
}

// Close releases the pipeline.
func (p *RecordProcessor) Close(ctx context.Context) error {
	return p.pipeline.Close(ctx)
}
