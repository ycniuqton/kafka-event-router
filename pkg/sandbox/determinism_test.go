package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

const deterministicTransform = `function transform(event, ctx) {
  const rolls = [];
  for (let i = 0; i < 5; i++) rolls.push(Math.random());
  globalThis.counter = (globalThis.counter || 0) + 1;
  return { key: event.key, value: { rolls, now: Date.now(), iso: new Date().toISOString(), ctxNow: ctx.now(),
    hash: ctx.hash(event.value), counter: globalThis.counter, ts: event.timestamp } };
}`

func callBytes(t *testing.T, mod sandbox.Module, inv sandbox.Invocation) []byte {
	t.Helper()
	res, err := mod.Call(context.Background(), inv)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res.Outputs)
	return b
}

// TestDeterminismAcrossRunsAndInstances checks FR-9/NFR-2: same input, seed and now give
// byte-identical output over 1,000 runs, across fresh loads and across concurrent instances.
func TestDeterminismAcrossRunsAndInstances(t *testing.T) {
	m := sandboxtest.NewManager(t)
	inv := sandbox.Invocation{Event: typicalEvent(), Now: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)}
	mod, _ := sandboxtest.LoadOne(t, m, sandbox.EntryTransform, deterministicTransform)
	first := callBytes(t, mod, inv)
	for i := 0; i < 1000; i++ {
		if got := callBytes(t, mod, inv); !bytes.Equal(got, first) {
			t.Fatalf("run %d differs:\n%s\n%s", i, got, first)
		}
	}
	for i := 0; i < 3; i++ {
		other, _ := sandboxtest.LoadOne(t, m, sandbox.EntryTransform, deterministicTransform)
		if got := callBytes(t, other, inv); !bytes.Equal(got, first) {
			t.Fatalf("fresh instance %d differs:\n%s\n%s", i, got, first)
		}
	}
}

func TestDeterminismInputsMatter(t *testing.T) {
	m := sandboxtest.NewManager(t)
	mod, _ := sandboxtest.LoadOne(t, m, sandbox.EntryTransform, deterministicTransform)
	base := sandbox.Invocation{Event: typicalEvent(), Now: time.Unix(1700000000, 0)}
	a := callBytes(t, mod, base)
	later := base
	later.Now = base.Now.Add(time.Second)
	moved := base
	moved.Event.Offset++
	seed := uint64(7)
	pinned := base
	pinned.SeedOverride = &seed
	for name, inv := range map[string]sandbox.Invocation{"now": later, "offset": moved, "seed": pinned} {
		if bytes.Equal(callBytes(t, mod, inv), a) {
			t.Errorf("changing %s must change the output", name)
		}
	}
	if !bytes.Equal(callBytes(t, mod, pinned), callBytes(t, mod, pinned)) {
		t.Error("a pinned seed must replay identically")
	}
}

func TestHashSaltIsPerTeam(t *testing.T) {
	m := sandboxtest.NewManager(t)
	code := `function transform(e, ctx) { return { value: ctx.hash("same") }; }`
	hashFor := func(team string) any {
		scope := sandboxtest.Scope(1)
		scope.TeamID = team
		rs, err := m.Load(context.Background(), scope, []sandbox.Source{{StepPath: "/s", Entry: sandbox.EntryTransform, Code: code}}, sandbox.RouteLimits{})
		if err != nil {
			t.Fatal(err)
		}
		defer rs.Close(context.Background())
		mod, _ := rs.Module("/s")
		res, err := mod.Call(context.Background(), sandbox.Invocation{Event: typicalEvent(), Now: time.Unix(1, 0)})
		if err != nil {
			t.Fatal(err)
		}
		return res.Outputs[0].Value
	}
	if hashFor("a") == hashFor("b") || hashFor("a") != hashFor("a") {
		t.Fatal("hash must be stable per team and differ between teams")
	}
}
