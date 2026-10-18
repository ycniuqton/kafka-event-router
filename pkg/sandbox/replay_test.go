package sandbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/boundary"
	"github.com/eventrouter/router/pkg/sandbox/determinism"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

const replayCode = `function transform(e, ctx) {
  return { value: { r: [Math.random(), Math.random()], now: ctx.now(), date: Date.now(), v: e.value } };
}`

// TestReplayUsesOriginalSeedPolicy (JS-204): replaying records on a later version in another process
// draws the same Math.random values; ctx.now() and Date are the replay's processing time.
func TestReplayUsesOriginalSeedPolicy(t *testing.T) {
	ctx := context.Background()
	original := runRecords(t, sandboxtest.NewManager(t), "v1", ctx, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	replayAt := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	replayed := runRecords(t, sandboxtest.NewManager(t), "v7", determinism.PassContext{}.With(ctx, determinism.PassReplay), replayAt)
	for i := range original {
		o, r := original[i], replayed[i]
		if o["r"].([]any)[0] != r["r"].([]any)[0] || o["r"].([]any)[1] != r["r"].([]any)[1] {
			t.Fatalf("record %d: replay random %v, original %v", i, r["r"], o["r"])
		}
		if r["now"] != "2024-03-01T12:00:00.000Z" || r["date"] != float64(replayAt.UnixMilli()) {
			t.Fatalf("record %d: replay clock %v / %v", i, r["now"], r["date"])
		}
	}
	if original[0]["r"].([]any)[0] == original[1]["r"].([]any)[0] {
		t.Fatal("different offsets must get different seeds")
	}
}

func runRecords(t *testing.T, m sandbox.Manager, version string, ctx context.Context, now time.Time) []map[string]any {
	t.Helper()
	scope := sandboxtest.Scope(1)
	scope.VersionID, scope.Mode = version, sandbox.ScopeWorker
	rs, err := m.Load(ctx, scope, []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: replayCode}}, sandbox.RouteLimits{})
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close(ctx)
	mod, _ := rs.Module("/s")
	var out []map[string]any
	for offset := range int64(3) {
		res, err := mod.Call(ctx, sandbox.Invocation{Event: boundary.GuestEvent{Topic: "orders", Partition: 1, Offset: 40 + offset, Value: offset}, Now: now})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, res.Outputs[0].Value.(map[string]any))
	}
	return out
}
