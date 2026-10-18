package engine_test

import (
	"context"
	"testing"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/enginetest"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

// BenchMetric names the custom metrics the S1 matrix reports next to ns/op, B/op and allocs/op.
type BenchMetric string

const metricEventsPerSec BenchMetric = "events/s"

// MatrixSandbox sizes the sandbox pool of JS shapes; the benchmark is single-goroutine.
type MatrixSandbox int

const matrixParallelism MatrixSandbox = 1

// pipelineCase runs decode → process → encode of one shape × payload cell.
type pipelineCase struct {
	shape   enginetest.BenchShape
	payload []byte
}

// options returns the golden compile options, with the real QuickJS runtime for JS shapes.
func (c pipelineCase) options(b *testing.B) engine.CompileOptions {
	opts := enginetest.GoldenOptions()
	if c.shape.NeedsJS {
		opts.Scripts = sandboxtest.WorkerScriptRuntime{Parallelism: int(matrixParallelism)}.Runtime(b)
	}
	return opts
}

// Run measures the cell and reports events/s.
func (c pipelineCase) Run(b *testing.B) {
	p := enginetest.MustCompileWith(b, c.shape.Route(), c.options(b))
	ctx := context.Background()
	b.SetBytes(int64(len(c.payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ev, err := p.Decode(ctx, engine.RawRecord{Value: c.payload, Topic: "in"})
		if err != nil {
			b.Fatal(err)
		}
		outs, err := p.Process(ctx, ev, nil)
		if err != nil || len(outs) != 1 {
			b.Fatalf("process: %d outputs, %v", len(outs), err)
		}
		if _, err := p.Encode(ctx, outs[0]); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), string(metricEventsPerSec))
}

// BenchmarkS1Engine is the S1 engine-only matrix: step type × payload size, one core, no Kafka.
func BenchmarkS1Engine(b *testing.B) {
	for _, shape := range enginetest.BenchShapes {
		b.Run("shape="+string(shape.Name), func(b *testing.B) {
			for _, size := range enginetest.BenchPayloadSizes {
				c := pipelineCase{shape: shape, payload: enginetest.OrderPayload{Size: size}.Bytes()}
				b.Run("payload="+size.Label(), c.Run)
			}
		})
	}
}
