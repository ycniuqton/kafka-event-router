package compile_test

import (
	"bytes"
	"testing"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

// fastRunner compiles route with the default (fast) codec and requires a fast plan.
func fastRunner(t *testing.T, route string) engine.RecordRunner {
	t.Helper()
	r := enginetest.MustCompile(t, route).(engine.RecordRunner)
	if !r.CodecPlan().Fast() {
		t.Fatalf("plan %s, want fast", r.CodecPlan())
	}
	return r
}

// fallbacks returns the tree re-runs counted by r.
func fallbacks(r engine.RecordRunner) uint64 {
	return r.(engine.CodecStatsReporter).CodecStats().Fallbacks
}

func TestProjectedRecordsFinishWithoutFallback(t *testing.T) {
	payload := enginetest.OrderPayload{Size: enginetest.Payload10KB}.Bytes()
	for _, shape := range enginetest.BenchShapes {
		if shape.NeedsJS || shape.Name == enginetest.ShapeMapper {
			continue
		}
		r := fastRunner(t, shape.Route())
		out, err := r.Run(ctx, engine.RawRecord{Value: payload, Topic: "in"})
		if err != nil || len(out) != 1 || fallbacks(r) != 0 {
			t.Fatalf("%s: %v, %d outputs, %d fallbacks", shape.Name, err, len(out), fallbacks(r))
		}
	}
}

func TestProjectedRecordsFallBackOnInvalidInput(t *testing.T) {
	r := fastRunner(t, enginetest.RouteJSON(`{"type":"cond","field":"value.a","op":"exists"}`, "[]"))
	for _, raw := range []string{`{"a":`, `[1]`, `"text"`, `null`} {
		_, _ = r.Run(ctx, engine.RawRecord{Value: []byte(raw)})
	}
	if fallbacks(r) != 4 {
		t.Fatalf("fallbacks %d, want 4", fallbacks(r))
	}
}

func TestProjectedRecordsPassthroughSharesInput(t *testing.T) {
	raw := []byte(`{"a":1,"b":{"c":[1,2.5,"x"]}}`)
	routes := []string{
		enginetest.RouteJSON(`{"type":"cond","field":"value.a","op":"exists"}`, "[]"),
		enginetest.RouteJSON("null", `[{"kind":"set","field":"value.a","source":{"from":"constant","value":1}}]`),
	}
	for _, route := range routes {
		out, err := fastRunner(t, route).Run(ctx, engine.RawRecord{Value: raw})
		if err != nil || len(out) != 1 || &out[0].Record.Value[0] != &raw[0] {
			t.Fatalf("unchanged canonical value must be the input bytes: %v %v", err, out)
		}
	}
}

func TestProjectedRecordsCanonicalizeUnsortedInput(t *testing.T) {
	r := fastRunner(t, enginetest.RouteJSON(`{"type":"cond","field":"value.b.c","op":"exists"}`, `[{"kind":"rename","from":"value.b.c","to":"value.b.d"}]`))
	out, err := r.Run(ctx, engine.RawRecord{Value: []byte(` {"z":1.50,"b":{"e":"é","c":[1]},"a":{"y":1,"x":2}} `)})
	want := `{"a":{"x":2,"y":1},"b":{"d":[1],"e":"é"},"z":1.5}`
	if err != nil || len(out) != 1 || !bytes.Equal(out[0].Record.Value, []byte(want)) || fallbacks(r) != 0 {
		t.Fatalf("got %s %v (fallbacks %d), want %s", out[0].Record.Value, err, fallbacks(r), want)
	}
}
