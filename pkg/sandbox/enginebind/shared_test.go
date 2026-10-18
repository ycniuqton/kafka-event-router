package enginebind_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/boundary"
	"github.com/eventrouter/router/pkg/sandbox/determinism"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

const doubled = "function transform(e) { return { value: { n: e.value.n * 2, r: Math.random() } }; }"

func TestRouteScriptsShareAcrossBranches(t *testing.T) {
	m := newManager(t)
	rt := enginebind.NewRouteScripts(m, sandboxtest.Scope(2))
	ctx := context.Background()
	lim := script.Limits{Timeout: 50 * time.Millisecond}
	a, err := rt.Compile(ctx, script.Source{Code: doubled, Entry: script.EntryTransform, Origin: "/branches/0/transform/steps/0/code"}, lim)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rt.Compile(ctx, script.Source{Code: doubled, Entry: script.EntryTransform, Origin: "/branches/1/transform/steps/0/code"}, lim)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Compile(ctx, script.Source{Code: "function transform( {", Entry: script.EntryTransform, Origin: "/bad"}, lim); err == nil {
		t.Fatal("compile error expected")
	}
	usage := rt.Usage()
	if len(usage) != 1 || usage[0].Modules != 1 {
		t.Fatalf("usage = %+v, want one sandbox with one shared module", usage)
	}
	for _, mod := range []script.Module{a, b} {
		res, err := mod.Call(ctx, script.Invocation{Event: engine.Event{Value: map[string]any{"n": 2.0}}.Snapshot()})
		if err != nil || res.Value.(map[string]any)["value"].(map[string]any)["n"] != 4.0 {
			t.Fatalf("call = %#v %v", res.Value, err)
		}
	}
	_ = a.Close(ctx)
	_ = a.Close(ctx)
	if len(rt.Usage()) != 1 {
		t.Fatal("a double close must not release the module of another branch")
	}
	_ = b.Close(ctx)
	if len(rt.Usage()) != 0 {
		t.Fatal("the sandbox closes after its last module")
	}
}

func TestRouteScriptsPinThroughContext(t *testing.T) {
	m := newManager(t)
	scope := sandboxtest.Scope(1)
	scope.Mode = sandbox.ScopeTest
	rt := enginebind.NewRouteScripts(m, scope)
	ctx := context.Background()
	mod, err := rt.Compile(ctx, script.Source{Code: doubled, Entry: script.EntryTransform, Origin: "/s"}, script.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer mod.Close(ctx)
	pin := determinism.Current(7, time.Unix(1, 0))
	call := func(ev engine.Event) any {
		res, err := mod.Call(determinism.PinContext{}.With(ctx, pin), script.Invocation{Event: ev.Snapshot(), Now: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		return res.Value.(map[string]any)["value"].(map[string]any)["r"]
	}
	if call(engine.Event{Offset: 1, Value: map[string]any{"n": 1.0}}) != call(engine.Event{Offset: 2, Value: map[string]any{"n": 1.0}}) {
		t.Fatal("pinned seed must make Math.random independent of the event position")
	}
	pin.Version++
	_, err = mod.Call(determinism.PinContext{}.With(ctx, pin), script.Invocation{Event: engine.Event{}.Snapshot()})
	if se, ok := sandbox.AsError(err); !ok || se.Kind != sandbox.ErrorDeterminism || errors.Is(err, script.ErrTimeout) {
		t.Fatalf("stale pin = %v", err)
	}
}

func TestScopeFormats(t *testing.T) {
	key := config.Format{Type: "string"}
	cfg := &config.RouteConfig{Source: config.Source{Format: config.Format{Type: "json"}, KeyFormat: &key},
		Branches: []config.Branch{{Destination: config.Destination{Format: &config.DestinationFormat{Type: "avro"}}}, {}}}
	got := enginebind.ScopeFormats{}.Of(cfg)
	want := []boundary.Format{boundary.FormatJSON, boundary.FormatString, boundary.FormatAvro}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("formats = %v", got)
	}
	if (enginebind.ScopeFormats{}).Of(nil) != nil {
		t.Fatal("nil config has no formats")
	}
}
