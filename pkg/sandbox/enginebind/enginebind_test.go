package enginebind_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/engines"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

type mapLookups map[string]map[string]any

func (m mapLookups) Lookup(table, key string) (any, bool) {
	v, ok := m[table][key]
	return v, ok
}

var tables = mapLookups{"countries": {"DE": "Germany", "42": "answer"}}

func newManager(t *testing.T) sandbox.Manager {
	t.Helper()
	m, err := sandbox.NewManager(context.Background(), sandbox.DefaultConfig(), sandbox.ManagerDeps{
		Engines: engines.Registry(),
		Salts:   sandboxtest.Salts{},
		Binder:  enginebind.Binder{Base: ctxhelpers.Set{Lookup: tables}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	return m
}

// TestHelperParityWithRecipes checks FR-5: every ctx helper returns exactly what the
// shared ctxhelpers implementation (used by recipe steps) returns.
func TestHelperParityWithRecipes(t *testing.T) {
	m := newManager(t)
	mod, _ := sandboxtest.LoadOne(t, m, sandbox.EntryTransform, `function transform(event, ctx) {
  const v = event.value;
  return { value: {
    email: v.inputs.map((x) => ctx.mask.email(x)),
    last4: v.inputs.map((x) => ctx.mask.last4(x)),
    full: v.inputs.map((x) => ctx.mask.full(x)),
    hash: v.inputs.map((x) => ctx.hash(x)),
    date: v.dates.map((d) => ctx.formatDate(d[0], d[1], d[2])),
    lookup: v.keys.map((k) => ctx.lookup('countries', k)),
  } };
}`)
	inputs := []any{"jane.doe@example.com", "4111111111111111", "ab", "", 42.0, "ünïcödé@x.de"}
	dates := [][]any{{"2024-01-02T03:04:05Z", "rfc3339", ""}, {"2024-01-02T03:04:05.123Z", "rfc3339_ms", "Europe/Berlin"},
		{1704164645000.0, "date", "America/New_York"}, {"2024-06-30T23:30:00Z", "datetime", "Asia/Tokyo"}}
	keys := []any{"DE", "FR", 42.0}
	res, err := mod.Call(context.Background(), sandbox.Invocation{
		Event: enginebind.SnapshotCodec{}.ToGuest(map[string]any{"value": map[string]any{"inputs": inputs, "dates": dates, "keys": keys}}),
		Now:   time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	got := res.Outputs[0].Value.(map[string]any)
	want := recipeOutputs(t, inputs, dates, keys)
	for name, w := range want {
		if fmt.Sprint(got[name]) != fmt.Sprint(w) {
			t.Errorf("%s:\n js     %v\n recipe %v", name, got[name], w)
		}
	}
}

func recipeOutputs(t *testing.T, inputs []any, dates [][]any, keys []any) map[string][]any {
	masker := ctxhelpers.NewStyleMasker(ctxhelpers.DefaultMaskPolicy())
	hasher := ctxhelpers.NewSaltedHasher(ctxhelpers.StaticSaltProvider{Value: []byte("salt-t1")}, "t1")
	out := map[string][]any{}
	for _, in := range inputs {
		for name, style := range map[string]config.MaskStyle{"email": config.MaskStyleEmail, "last4": config.MaskStyleLast4, "full": config.MaskStyleFull} {
			v, _ := masker.Mask(in, style)
			out[name] = append(out[name], v)
		}
		h, _ := hasher.Hash(context.Background(), in)
		out["hash"] = append(out["hash"], h)
	}
	for _, d := range dates {
		ts := parseDate(t, d[0])
		s, err := ctxhelpers.LayoutDateFormatter{}.Format(ts, ctxhelpers.DateLayout(d[1].(string)), d[2].(string))
		if err != nil {
			t.Fatal(err)
		}
		out["date"] = append(out["date"], s)
	}
	for _, k := range keys {
		v, _ := tables.Lookup("countries", fmt.Sprint(k))
		out["lookup"] = append(out["lookup"], v)
	}
	return out
}

func parseDate(t *testing.T, v any) time.Time {
	if s, ok := v.(string); ok {
		ts, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		return ts
	}
	return time.UnixMilli(int64(v.(float64))).UTC()
}

func TestScriptRuntimeAdapter(t *testing.T) {
	m := newManager(t)
	rt := enginebind.ScriptRuntime{Manager: m, Scope: sandboxtest.Scope(1)}
	ctx := context.Background()
	in := engine.Event{Key: "k", Value: map[string]any{"n": 2.0}, Topic: "orders", Partition: 1, Offset: 9,
		Headers: []engine.Header{{Key: "h", Value: []byte("v")}}, Timestamp: time.Unix(5, 0)}
	mod, err := rt.Compile(ctx, script.Source{Code: `function transform(e) {
  return [{ ...e, value: { n: e.value.n * 2, h: e.headers.h, topic: e.topic, offset: e.offset } }, { value: 'second' }];
}`, Entry: script.EntryTransform, Origin: "/branches/0/transform/steps/0/code"}, script.Limits{Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer mod.Close(ctx)
	res, err := mod.Call(ctx, script.Invocation{Event: in.Snapshot(), Now: time.Unix(6, 0)})
	if err != nil {
		t.Fatal(err)
	}
	list, ok := res.Value.([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("value = %#v", res.Value)
	}
	first := in.WithSnapshot(list[0].(map[string]any))
	want := map[string]any{"n": 4.0, "h": "v", "topic": "orders", "offset": 9.0}
	if fmt.Sprint(first.Value) != fmt.Sprint(want) || first.Key != "k" || string(first.Headers[0].Value) != "v" {
		t.Fatalf("first = %+v", first)
	}
}

func TestScriptRuntimePredicateAndErrors(t *testing.T) {
	m := newManager(t)
	rt := enginebind.ScriptRuntime{Manager: m, Scope: sandboxtest.Scope(1)}
	ctx := context.Background()
	pred, err := rt.Compile(ctx, script.Source{Code: "event.value.n > 1", Entry: script.EntryPredicate, Origin: "/f"}, script.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer pred.Close(ctx)
	res, err := pred.Call(ctx, script.Invocation{Event: engine.Event{Value: map[string]any{"n": 2.0}}.Snapshot()})
	if err != nil || res.Value != true {
		t.Fatalf("predicate = %v, %v", res.Value, err)
	}
	_, err = rt.Compile(ctx, script.Source{Code: "function transform(e) {\n  return ;;\n  }}", Entry: script.EntryTransform, Origin: "/x"}, script.Limits{})
	var ce *script.CompileError
	if !errors.As(err, &ce) || ce.Line != 3 {
		t.Fatalf("compile error = %#v", err)
	}
	loop, err := rt.Compile(ctx, script.Source{Code: "function transform(){ for(;;){} }", Entry: script.EntryTransform, Origin: "/l"}, script.Limits{Timeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close(ctx)
	_, err = loop.Call(ctx, script.Invocation{Event: engine.Event{}.Snapshot()})
	if se, ok := sandbox.AsError(err); !errors.Is(err, script.ErrTimeout) || !ok || se.Kind != sandbox.ErrorTimeout {
		t.Fatalf("timeout err = %v", err)
	}
}
