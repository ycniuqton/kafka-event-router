package compile_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/codec/codectest"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/enginetest"
)

const multiBranch = `{"apiVersion":"router/v1","name":"mb","source":{"connectionId":"c","topics":["in"],"format":{"type":"json"}},
 "branches":[
  {"id":"a","transform":{"steps":[{"kind":"set","field":"value.x","source":{"from":"constant","value":"a"}}]},"destination":{"connectionId":"c","topic":"ta"}},
  {"id":"b","filter":{"type":"cond","field":"value.x","op":"not_exists"},"transform":{"steps":[{"kind":"remove","fields":["value.n.k"]}]},"destination":{"connectionId":"c","topic":"tb"}},
  {"id":"c","filter":{"type":"cond","field":"value.n.k","op":"exists"},"destination":{"connectionId":"c","topic":"tc"}}
 ]}`

func TestBranchesAreIsolated(t *testing.T) {
	p := enginetest.MustCompile(t, multiBranch)
	outs, err := p.Process(ctx, enginetest.MustDecode(t, p, `{"n":{"k":1}}`), nil)
	if err != nil || len(outs) != 3 {
		t.Fatalf("%v %+v", err, outs)
	}
	want := map[engine.BranchID]any{
		"a": map[string]any{"n": map[string]any{"k": int64(1)}, "x": "a"},
		"b": map[string]any{"n": map[string]any{}},
		"c": map[string]any{"n": map[string]any{"k": int64(1)}},
	}
	for _, o := range outs {
		if !reflect.DeepEqual(o.Value, want[o.Branch]) {
			t.Errorf("branch %s: %#v", o.Branch, o.Value)
		}
		rec, err := p.Encode(ctx, o)
		if err != nil || rec.Topic != "t"+string(o.Branch) {
			t.Errorf("encode %s: %v %s", o.Branch, err, rec.Topic)
		}
	}
}

func TestDuplicateBranchIDs(t *testing.T) {
	route := `{"apiVersion":"router/v1","name":"d","source":{"connectionId":"c","topics":["in"],"format":{"type":"json"}},
	 "branches":[{"id":"x","destination":{"connectionId":"c","topic":"a"}},{"id":"x","destination":{"connectionId":"c","topic":"b"}}]}`
	doc, _ := config.Parse([]byte(route))
	_, err := compile.NewDefaultCompiler().Compile(ctx, doc, enginetest.GoldenOptions())
	ce, ok := err.(*engine.CompileError)
	if !ok || len(ce.Issues) != 1 || ce.Issues[0].Path != "/branches/1/id" {
		t.Fatalf("%v", err)
	}
}

const avroRoute = `{"apiVersion":"router/v1","name":"av","source":{"connectionId":"src","topics":["orders"],"format":{"type":"avro"}},
 "branches":[{"id":"main","transform":{"steps":[{"kind":"set","field":"value.total","source":{"from":"formula","expr":"value.qty * 2"}}]},
  "destination":{"connectionId":"dst","topic":"orders-out","format":{"type":"avro"}}}]}`

const inSchema = `{"type":"record","name":"In","fields":[{"name":"qty","type":"long"},{"name":"at","type":{"type":"long","logicalType":"timestamp-millis"}}]}`
const outSchema = `{"type":"record","name":"Out","fields":[{"name":"qty","type":"long"},{"name":"total","type":"long"},{"name":"at","type":{"type":"long","logicalType":"timestamp-millis"}}]}`

// providerOf serves one fake registry per connection id.
type providerOf map[string]*codectest.MemoryRegistry

func (r providerOf) Registry(_ context.Context, id string) (codec.SchemaRegistry, error) {
	return r[id], nil
}

func TestAvroPipelineRoundTrip(t *testing.T) {
	src, dst := codectest.NewMemoryRegistry(), codectest.NewMemoryRegistry()
	src.Register("orders-value", codec.SchemaTypeAvro, inSchema)
	dst.Register("x", codec.SchemaTypeAvro, `"string"`)
	outID := dst.Register("orders-out-value", codec.SchemaTypeAvro, outSchema)
	opts := enginetest.GoldenOptions()
	opts.SchemaRegistries = providerOf{"src": src, "dst": dst}
	doc, _ := config.Parse([]byte(avroRoute))
	p, err := compile.NewDefaultCompiler().Compile(ctx, doc, opts)
	if err != nil {
		t.Fatal(err)
	}
	subject := "orders-value"
	in, _ := codec.DefaultFactoryManager().NewSpec(codec.CodecSpec{Format: config.Format{Type: config.FormatTypeAvro, Subject: &subject}, Registry: src})
	raw, err := in.Encode(ctx, map[string]any{"qty": int64(3), "at": "2026-01-01T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	ev, err := p.Decode(ctx, engine.RawRecord{Value: raw})
	if err != nil {
		t.Fatal(err)
	}
	outs, err := p.Process(ctx, ev, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := p.Encode(ctx, outs[0])
	if err != nil {
		t.Fatal(err)
	}
	frame, _ := codec.Framing{}.Parse(rec.Value)
	if frame.SchemaID != outID {
		t.Fatalf("encoded with schema %d, want %d (dest subject <topic>-value)", frame.SchemaID, outID)
	}
	src.Down = true
	fresh, _ := compile.NewDefaultCompiler().Compile(ctx, doc, opts)
	if _, err := fresh.Decode(ctx, engine.RawRecord{Value: raw}); !isType(err, engine.ErrDecodeTransient) {
		t.Fatalf("registry down: %v", err)
	}
	none, _ := compile.NewDefaultCompiler().Compile(ctx, doc, enginetest.GoldenOptions())
	if _, err := none.Decode(ctx, engine.RawRecord{Value: raw}); !isType(err, engine.ErrDecode) {
		t.Fatalf("no registry: %v", err)
	}
}
