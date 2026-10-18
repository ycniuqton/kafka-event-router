package sandbox_test

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/engine/enginetest"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/boundary"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

// typicalTransform is the 20-line transform of the CI performance budget (FR-17).
const typicalTransform = `function transform(event, ctx) {
  const order = event.value.order;
  if (!order || !Array.isArray(order.items)) {
    return null;
  }
  let total = 0;
  let count = 0;
  for (const item of order.items) {
    total += item.price * item.qty;
    count += item.qty;
  }
  const tags = order.items.map((i) => i.sku.toLowerCase());
  return {
    key: order.id,
    value: {
      orderId: order.id,
      customer: order.customer.name.toUpperCase(),
      total: Math.round(total * 100) / 100,
      count: count,
      tags: tags,
      large: total > 100,
    },
  };
}`

func typicalEvent() boundary.GuestEvent {
	items := []any{
		map[string]any{"sku": "A-1", "price": 12.5, "qty": 2},
		map[string]any{"sku": "B-2", "price": 99.99, "qty": 1},
		map[string]any{"sku": "C-3", "price": 3.1, "qty": 7},
	}
	order := map[string]any{"id": "o-1", "customer": map[string]any{"name": "Ann", "email": "ann@x.com"}, "items": items}
	return boundary.GuestEvent{Value: map[string]any{"order": order}, Topic: "orders", Partition: 1, Offset: 42,
		Headers: []boundary.Header{{Key: "trace", Value: []byte("abc")}}, Timestamp: time.Unix(1700000000, 0)}
}

func BenchmarkTypicalTransform(b *testing.B) {
	m := sandboxtest.NewManager(b)
	mod, _ := sandboxtest.LoadOne(b, m, sandbox.EntryTransform, typicalTransform)
	inv := sandbox.Invocation{Event: typicalEvent(), Now: time.Unix(1700000001, 0)}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := mod.Call(ctx, inv); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInstanceCreation(b *testing.B) {
	m := sandboxtest.NewManager(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rs, err := m.Load(ctx, sandboxtest.Scope(1), []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: typicalTransform}}, sandbox.RouteLimits{})
		if err != nil {
			b.Fatal(err)
		}
		_ = rs.Close(ctx)
	}
}

// PerfBudget holds the CI performance budget of FR-17 / NFR-3.
type PerfBudget time.Duration

const (
	budgetTypicalCallP50 PerfBudget = PerfBudget(500 * time.Microsecond)
	budgetInstanceP50    PerfBudget = PerfBudget(5 * time.Millisecond)
)

// PerfGate configures the performance gate run.
type PerfGate string

const (
	perfGateEnv PerfGate = "ROUTER_SANDBOX_PERF_GATE"
	perfSamples int      = 400
)

// TestPerformanceBudget is the CI regression gate; it runs only when ROUTER_SANDBOX_PERF_GATE=1
// because shared developer machines are too noisy for a hard threshold.
func TestPerformanceBudget(t *testing.T) {
	if os.Getenv(string(perfGateEnv)) == "" {
		t.Skipf("set %s=1 to enforce the sandbox performance budget", perfGateEnv)
	}
	m := sandboxtest.NewManager(t)
	mod, _ := sandboxtest.LoadOne(t, m, sandbox.EntryTransform, typicalTransform)
	inv := sandbox.Invocation{Event: typicalEvent(), Now: time.Unix(1700000001, 0)}
	calls := sampleP50(t, func() error { _, err := mod.Call(context.Background(), inv); return err })
	loads := sampleP50(t, func() error {
		rs, err := m.Load(context.Background(), sandboxtest.Scope(1), []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: typicalTransform}}, sandbox.RouteLimits{})
		if err == nil {
			_ = rs.Close(context.Background())
		}
		return err
	})
	t.Logf("typical call p50 %v, load+instance p50 %v", calls, loads)
	if calls > time.Duration(budgetTypicalCallP50) || loads > time.Duration(budgetInstanceP50) {
		t.Fatalf("budget exceeded: call p50 %v (max %v), instance p50 %v (max %v)", calls,
			time.Duration(budgetTypicalCallP50), loads, time.Duration(budgetInstanceP50))
	}
}

func sampleP50(t *testing.T, fn func() error) time.Duration {
	t.Helper()
	samples := make([]time.Duration, 0, perfSamples)
	for i := 0; i < perfSamples; i++ {
		start := time.Now()
		if err := fn(); err != nil {
			t.Fatal(err)
		}
		samples = append(samples, time.Since(start))
	}
	slices.Sort(samples)
	return samples[len(samples)/2]
}

// sandboxCallCase measures one JS call on the S1 order payload of one size: boundary
// conversion in and out plus QuickJS execution, without engine decode/encode.
type sandboxCallCase struct {
	event boundary.GuestEvent
}

// newSandboxCallCase decodes the payload into the guest event shape the engine hands over.
func newSandboxCallCase(b *testing.B, size enginetest.PayloadSize) sandboxCallCase {
	var value map[string]any
	if err := json.Unmarshal(enginetest.OrderPayload{Size: size}.Bytes(), &value); err != nil {
		b.Fatal(err)
	}
	return sandboxCallCase{event: boundary.GuestEvent{Value: value, Topic: "in", Timestamp: time.Unix(1700000000, 0)}}
}

// Run calls the transform b.N times and reports events/s.
func (c sandboxCallCase) Run(b *testing.B) {
	m := sandboxtest.NewManager(b)
	rs, err := m.Load(context.Background(), sandboxtest.Scope(1), []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: enginetest.BenchTransformJS}},
		sandbox.RouteLimits{JSTimeoutMs: int64(enginetest.BenchJSTimeoutMs)})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = rs.Close(context.Background()) })
	mod, _ := rs.Module("/s")
	inv := sandbox.Invocation{Event: c.event, Now: time.Unix(1700000001, 0)}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := mod.Call(ctx, inv); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "events/s")
}

// BenchmarkS1SandboxCall is the JS row of the S1 matrix at sandbox level, per payload size.
func BenchmarkS1SandboxCall(b *testing.B) {
	for _, size := range enginetest.BenchPayloadSizes {
		b.Run("payload="+size.Label(), newSandboxCallCase(b, size).Run)
	}
}
