package compile

import (
	"context"
	"sync/atomic"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compilectx"
	"github.com/eventrouter/router/pkg/engine/projection"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// RecordStrategy runs one raw record through decode → process → encode.
type RecordStrategy interface {
	Run(ctx context.Context, p *compiledPipeline, r engine.RawRecord) ([]engine.Encoded, error)
}

// Run implements engine.RecordRunner with the strategy chosen at compile.
func (p *compiledPipeline) Run(ctx context.Context, r engine.RawRecord) ([]engine.Encoded, error) {
	return p.records.Run(ctx, p, r)
}

// CodecPlan implements engine.RecordRunner.
func (p *compiledPipeline) CodecPlan() engine.CodecPlan { return p.plan }

// CodecStats implements engine.CodecStatsReporter.
func (p *compiledPipeline) CodecStats() engine.CodecStats {
	return engine.CodecStats{Fallbacks: p.fallbacks.Load()}
}

// branchOf returns the compiled branch with id, or nil.
func (p *compiledPipeline) branchOf(id engine.BranchID) *branch {
	for _, b := range p.branches {
		if b.id == id {
			return b
		}
	}
	return nil
}

// CodecInputs collects what the analyser needs from a compiled pipeline.
type CodecInputs struct{}

// Of returns the analyser inputs of p.
func (CodecInputs) Of(p *compiledPipeline, cfg *config.RouteConfig, opts engine.CompileOptions, scope *compilectx.Scope) projection.Inputs {
	in := projection.Inputs{Mode: opts.Codec, SourceFormat: cfg.Source.Format.Type, Access: scope.Access}
	for _, b := range p.branches {
		in.DestinationFormats = append(in.DestinationFormats, b.output.ValueFormat())
	}
	return in
}

// RecordStrategies binds a codec plan to its record strategy.
type RecordStrategies struct{}

// Assign stores plan and its strategy on p.
func (RecordStrategies) Assign(p *compiledPipeline, plan projection.Plan) {
	p.plan = plan.CodecPlan
	p.records = TreeRecords{}
	if plan.Fast() {
		p.records = ProjectedRecords{codec: projection.NewCodec(plan.Fields), fallbacks: &p.fallbacks}
	}
}

// TreeRecords runs Decode, Process (no tracer) and Encode of every output.
type TreeRecords struct{}

func (TreeRecords) Run(ctx context.Context, p *compiledPipeline, r engine.RawRecord) ([]engine.Encoded, error) {
	ev, err := p.Decode(ctx, r)
	if err != nil {
		return nil, err
	}
	outs, err := p.Process(ctx, ev, trace.NoopTracer{})
	if err != nil {
		return nil, err
	}
	encoded := make([]engine.Encoded, 0, len(outs))
	for _, o := range outs {
		rec, err := p.Encode(ctx, o)
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, engine.Encoded{Record: rec, Branch: o.Branch})
	}
	return encoded, nil
}

// ProjectedRecords decodes only the planned fields and patches outputs into the input
// bytes; any record it cannot finish (not an object, any error) re-runs on TreeRecords,
// so results and errors are always the tree path's.
type ProjectedRecords struct {
	codec     *projection.Codec
	fallbacks *atomic.Uint64
}

func (s ProjectedRecords) Run(ctx context.Context, p *compiledPipeline, r engine.RawRecord) ([]engine.Encoded, error) {
	if out, ok := s.fast(ctx, p, r); ok {
		return out, nil
	}
	s.fallbacks.Add(1)
	return TreeRecords{}.Run(ctx, p, r)
}

func (s ProjectedRecords) fast(ctx context.Context, p *compiledPipeline, r engine.RawRecord) ([]engine.Encoded, bool) {
	doc, value, ok := s.codec.Decode(r.Value)
	if !ok {
		return nil, false
	}
	defer doc.Release()
	key, err := p.source.Key.Decode(ctx, r.Key)
	if err != nil {
		return nil, false
	}
	ev := engine.Event{Key: key, Value: value, Headers: r.Headers, Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp}
	outs, err := p.Process(ctx, ev, trace.NoopTracer{})
	if err != nil {
		return nil, false
	}
	return s.encode(ctx, p, doc, outs)
}

func (ProjectedRecords) encode(ctx context.Context, p *compiledPipeline, doc *projection.Document, outs []engine.Out) ([]engine.Encoded, bool) {
	encoded := make([]engine.Encoded, 0, len(outs))
	for _, o := range outs {
		b := p.branchOf(o.Branch)
		if b == nil {
			return nil, false
		}
		rec, err := b.output.EncodeWith(ctx, o, p.limits.MaxOutputBytes, ProjectedValue{Doc: doc, State: b.valueState()})
		if err != nil {
			return nil, false
		}
		encoded = append(encoded, engine.Encoded{Record: rec, Branch: o.Branch})
	}
	return encoded, true
}

// valueState reports whether the branch can change the value (it has steps).
func (b *branch) valueState() projection.ValueState {
	if b.chain.Len() == 0 {
		return projection.ValueUntouched
	}
	return projection.ValueEdited
}

// ProjectedValue encodes output values against the decoded document (nil = tombstone).
type ProjectedValue struct {
	Doc   *projection.Document
	State projection.ValueState
}

func (v ProjectedValue) Encode(_ context.Context, value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return v.Doc.Encode(value, v.State)
}
