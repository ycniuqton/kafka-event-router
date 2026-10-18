package runner_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/enginetest"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

// BenchmarkRecordProcessorFilterOnly measures decode → filter → encode per core, with p95 latency.
func BenchmarkRecordProcessorFilterOnly(b *testing.B) {
	c := cluster(b)
	v := compiled(b, c, "dlq")
	p := runner.NewRecordProcessor(v)
	raw := engine.RawRecord{Key: []byte("k"), Value: []byte(`{"keep":true,"n":1,"name":"order"}`), Topic: "in"}
	lat := make([]time.Duration, 0, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if _, err := p.Process(context.Background(), raw); err != nil {
			b.Fatal(err)
		}
		lat = append(lat, time.Since(start))
	}
	b.StopTimer()
	slices.Sort(lat)
	b.ReportMetric(float64(lat[len(lat)*95/100].Nanoseconds()), "p95-ns")
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "events/s")
}

// BenchmarkRunnerEndToEnd measures consume → filter → produce → commit through an in-memory cluster.
func BenchmarkRunnerEndToEnd(b *testing.B) {
	c := cluster(b)
	vals := make([]string, b.N)
	for i := range vals {
		vals[i] = fmt.Sprintf(`{"keep":%t,"n":%d}`, i%2 == 0, i)
	}
	produce(b, c, vals...)
	m := newManager(&haltRecorder{})
	b.ResetTimer()
	if err := m.Start(context.Background(), compiled(b, c, "dlq")); err != nil {
		b.Fatal(err)
	}
	eventually(b, "commit", func() bool { return committed(b, c, "router.r1") == int64(b.N) })
	b.StopTimer()
	m.StopAll(context.Background())
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "events/s")
}

// ProcessorSandbox sizes the sandbox pool of JS shapes; the benchmark is single-goroutine.
type ProcessorSandbox int

const processorParallelism ProcessorSandbox = 1

// processorCase measures RecordProcessor.Process (decode → pipeline → encode → destination
// binding, the worker's per-record CPU path) for one S1 shape × payload cell.
type processorCase struct {
	shape enginetest.BenchShape
	raw   engine.RawRecord
}

// processor compiles the shape as the worker does, with the real sandbox for JS shapes.
func (c processorCase) processor(b *testing.B) *runner.RecordProcessor {
	opts := engine.CompileOptions{}
	if c.shape.NeedsJS {
		opts.Scripts = sandboxtest.WorkerScriptRuntime{Parallelism: int(processorParallelism)}.Runtime(b)
	}
	doc, err := config.Parse([]byte(c.shape.Route()))
	if err != nil {
		b.Fatal(err)
	}
	cfg, err := doc.Config()
	if err != nil {
		b.Fatal(err)
	}
	p := enginetest.MustCompileWith(b, c.shape.Route(), opts)
	return runner.NewRecordProcessor(runner.CompiledVersion{Ref: domain.VersionRef{RouteID: "r1", VersionID: "v1", VersionNo: 1}, Config: cfg, Pipeline: p})
}

// Run measures the cell and reports events/s.
func (c processorCase) Run(b *testing.B) {
	p := c.processor(b)
	ctx := context.Background()
	b.SetBytes(int64(len(c.raw.Value)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		outs, err := p.Process(ctx, c.raw)
		if err != nil || len(outs) != 1 {
			b.Fatalf("process: %d outputs, %v", len(outs), err)
		}
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "events/s")
}

// BenchmarkS1RecordProcessor is the S1 matrix at worker level for the headline shapes.
func BenchmarkS1RecordProcessor(b *testing.B) {
	shapes := enginetest.BenchShapeSet(enginetest.BenchShapes).Only(enginetest.ShapeFilterOnly, enginetest.ShapeRecipe5, enginetest.ShapeMapper, enginetest.ShapeJS)
	for _, shape := range shapes {
		b.Run("shape="+string(shape.Name), func(b *testing.B) {
			for _, size := range enginetest.BenchPayloadSizes {
				raw := engine.RawRecord{Key: []byte("k"), Value: enginetest.OrderPayload{Size: size}.Bytes(), Topic: "in"}
				b.Run("payload="+size.Label(), processorCase{shape: shape, raw: raw}.Run)
			}
		})
	}
}
