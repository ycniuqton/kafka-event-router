package sandbox_test

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

// BenchmarkJSPoolInstances measures the throughput of one JS step under more concurrent callers than
// instances (12 partition lanes) with a fixed pool of n instances: the B3 cap of the stress test was n = 4.
func BenchmarkJSPoolInstances(b *testing.B) {
	for _, n := range []int{1, 2, 4, 6, 8, 12} {
		if n > 2*runtime.GOMAXPROCS(0) {
			continue
		}
		b.Run(fmt.Sprintf("instances=%d", n), func(b *testing.B) {
			m := sandboxtest.NewManager(b)
			scope := workerScope(n, sandbox.PoolRequest{Min: n, Max: n})
			rs, err := m.Load(context.Background(), scope, []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: typicalTransform}}, sandbox.RouteLimits{})
			if err != nil {
				b.Fatal(err)
			}
			defer rs.Close(context.Background())
			_, _ = m.Prewarm(context.Background(), scope)
			mod, _ := rs.Module("/s")
			inv := sandbox.Invocation{Event: typicalEvent(), Now: time.Unix(1700000001, 0)}
			b.SetParallelism(max(1, 12/runtime.GOMAXPROCS(0)+1))
			b.ResetTimer()
			began := time.Now()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if _, err := mod.Call(context.Background(), inv); err != nil {
						b.Error(err)
						return
					}
				}
			})
			b.ReportMetric(float64(b.N)/time.Since(began).Seconds(), "calls/s")
		})
	}
}
