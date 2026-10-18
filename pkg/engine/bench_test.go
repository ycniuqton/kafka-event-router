package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

// benchGateEnv enables the CI threshold gate (TestBenchThresholds).
const benchGateEnv = "ROUTER_BENCH_GATE"

// benchEvent is a ~1 KB JSON order event.
var benchEvent = func() []byte {
	items := []string{}
	for i := 0; i < 8; i++ {
		items = append(items, fmt.Sprintf(`{"sku":"SKU-%03d","qty":%d,"price":%d.99,"name":"Item number %d with a name"}`, i, i+1, 10+i, i))
	}
	return []byte(`{"eventType":"OrderPaid","amount":250.5,"currency":"EUR","cust_id":"C-123456","email":"jane.doe@example.com",
"card":"4111111111111111","orderId":"o-987654321","createdAt":"2025-12-31T12:00:00Z","items":[` + strings.Join(items, ",") + `]}`)
}()

var benchFilter = `{"type":"and","children":[{"type":"cond","field":"value.eventType","op":"eq","value":"OrderPaid"},
{"type":"cond","field":"value.amount","op":"gt","value":100},{"type":"cond","field":"value.currency","op":"in","values":["EUR","USD"]}]}`

var benchSteps = `[{"kind":"rename","from":"value.cust_id","to":"value.customerId"},{"kind":"mask","field":"value.email","style":"email"},
{"kind":"mask","field":"value.card","style":"last4"},{"kind":"set","field":"value.source","source":{"from":"constant","value":"router"}},
{"kind":"remove","fields":["value.createdAt"]},{"kind":"convert_type","field":"value.amount","targetType":"string"},
{"kind":"set_key","source":{"from":"field","path":"value.orderId"}},{"kind":"default_if_missing","field":"value.region","value":"eu"}]`

func benchPipeline(b *testing.B, filter, steps string) {
	p := enginetest.MustCompile(b, enginetest.RouteJSON(filter, steps))
	ctx := context.Background()
	b.SetBytes(int64(len(benchEvent)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ev, err := p.Decode(ctx, engine.RawRecord{Value: benchEvent})
		if err != nil {
			b.Fatal(err)
		}
		outs, err := p.Process(ctx, ev, nil)
		if err != nil || len(outs) != 1 {
			b.Fatal(err)
		}
		if _, err := p.Encode(ctx, outs[0]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFilterOnly(b *testing.B) { benchPipeline(b, benchFilter, `[]`) }

func BenchmarkRecipeChain(b *testing.B) { benchPipeline(b, benchFilter, benchSteps) }

// benchMapper reshapes the order with field, constant, formula and metadata leaves.
var benchMapper = enginetest.BenchMapper

func BenchmarkMapper(b *testing.B) { benchPipeline(b, benchFilter, benchMapper) }

// benchThreshold is the minimum throughput a benchmark must reach in CI.
type benchThreshold struct {
	MinEventsPerSec float64 `json:"minEventsPerSec"`
}

func TestBenchThresholds(t *testing.T) {
	if os.Getenv(benchGateEnv) == "" {
		t.Skip("set " + benchGateEnv + "=1 to run the benchmark regression gate")
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "bench_thresholds.json"))
	if err != nil {
		t.Fatal(err)
	}
	var thresholds map[string]benchThreshold
	if err := json.Unmarshal(raw, &thresholds); err != nil {
		t.Fatal(err)
	}
	benches := map[string]func(*testing.B){"BenchmarkFilterOnly": BenchmarkFilterOnly, "BenchmarkRecipeChain": BenchmarkRecipeChain, "BenchmarkMapper": BenchmarkMapper}
	for name, th := range thresholds {
		fn, ok := benches[name]
		if !ok {
			t.Fatalf("unknown benchmark %s", name)
		}
		res := testing.Benchmark(fn)
		perSec := float64(res.N) / res.T.Seconds()
		t.Logf("%s: %.0f events/s (min %.0f)", name, perSec, th.MinEventsPerSec)
		if perSec < th.MinEventsPerSec {
			t.Errorf("%s regressed: %.0f events/s < %.0f", name, perSec, th.MinEventsPerSec)
		}
	}
}
