package preview_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/preview"
)

// benchRoute is a recipe route over the demo order events: filter, rename, mask and remove (NFR-3).
const benchRoute = `{
  "apiVersion": "router/v1", "name": "bench-recipes",
  "source": { "connectionId": "c1", "topics": ["order-events"], "format": { "type": "json" } },
  "branches": [{
    "id": "main",
    "filter": { "type": "and", "children": [
      { "type": "cond", "field": "value.eventType", "op": "in", "values": ["order.paid", "order.created"] },
      { "type": "cond", "field": "value.amount", "op": "gt", "value": 20 }
    ] },
    "transform": { "steps": [
      { "id": "r1", "kind": "rename", "from": "value.cust_id", "to": "value.customerId" },
      { "id": "m1", "kind": "mask", "field": "value.email", "style": "email" },
      { "id": "x1", "kind": "remove", "fields": ["value.createdAt"] }
    ] },
    "destination": { "connectionId": "c1", "topic": "orders-public" }
  }]
}`

// LatencyBudget holds the NFR-3 targets checked by TestPreviewLatencyBudget.
type LatencyBudget struct {
	Samples int
	Limit   time.Duration
}

var (
	fullRunBudget = LatencyBudget{Samples: 1000, Limit: 2 * time.Second}
	rerunBudget   = LatencyBudget{Samples: 50, Limit: 300 * time.Millisecond}
	rerunRounds   = 40
)

func benchPlan(t testing.TB, samples int) preview.RunPlan {
	t.Helper()
	res, err := preview.NewDemoSource(func() time.Time { return demoNow }).Load(context.Background(), preview.LoadRequest{Mode: preview.ModeDemo, Count: samples}, newBudget())
	if err != nil || len(res.Records) != samples {
		t.Fatalf("demo samples %d err %v", len(res.Records), err)
	}
	return preview.RunPlan{Config: preview.RawConfig(benchRoute), Records: res.Records, Loaded: samples, Scope: preview.RunScope{TeamID: "t1", Mode: preview.RunPreview}}
}

func timedRun(t testing.TB, r preview.PreviewRunner, plan preview.RunPlan) time.Duration {
	t.Helper()
	start := time.Now()
	res, err := r.Run(context.Background(), plan)
	took := time.Since(start)
	if err != nil || res.Summary.Incomplete || res.Summary.Errors != 0 || res.Summary.Passed == 0 {
		t.Fatalf("summary %+v issues %v err %v", res.Summary, res.Issues, err)
	}
	return took
}

// TestPreviewLatencyBudget: 1,000 samples on a recipe route < 2 s; 50-sample re-runs p95 < 300 ms.
func TestPreviewLatencyBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("latency budget runs without -short")
	}
	r := newRunner(t)
	if took := timedRun(t, r, benchPlan(t, fullRunBudget.Samples)); took > fullRunBudget.Limit {
		t.Fatalf("1,000 samples took %s (budget %s)", took, fullRunBudget.Limit)
	}
	plan := benchPlan(t, rerunBudget.Samples)
	runs := make([]time.Duration, 0, rerunRounds)
	for range rerunRounds {
		runs = append(runs, timedRun(t, r, plan))
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i] < runs[j] })
	p95 := runs[len(runs)*95/100]
	t.Logf("50 samples p95 %s, max %s", p95, runs[len(runs)-1])
	if p95 > rerunBudget.Limit {
		t.Fatalf("50 samples p95 %s (budget %s)", p95, rerunBudget.Limit)
	}
}

func benchmarkRun(b *testing.B, samples int) {
	r := newRunner(b)
	plan := benchPlan(b, samples)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.Run(context.Background(), plan); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPreviewRun50(b *testing.B)   { benchmarkRun(b, rerunBudget.Samples) }
func BenchmarkPreviewRun1000(b *testing.B) { benchmarkRun(b, fullRunBudget.Samples) }
