package compile_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/config/configtest"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/enginetest"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// countingLibrary counts resolver calls to prove one load per pin per compile.
type countingLibrary struct {
	*configtest.InMemoryLibrary
	transforms, tables int
}

func (c *countingLibrary) Transform(ctx context.Context, ref string, v int) (config.Steps, error) {
	c.transforms++
	return c.InMemoryLibrary.Transform(ctx, ref, v)
}

func (c *countingLibrary) LookupTable(ctx context.Context, ref string, v int) (config.LookupRows, error) {
	c.tables++
	return c.InMemoryLibrary.LookupTable(ctx, ref, v)
}

// failingLibrary simulates an unreachable library store.
type failingLibrary struct{}

func (failingLibrary) Transform(context.Context, string, int) (config.Steps, error) {
	return nil, errors.New("connection refused")
}

func (failingLibrary) LookupTable(context.Context, string, int) (config.LookupRows, error) {
	return nil, errors.New("connection refused")
}

func steps(t *testing.T, raw string) config.Steps {
	t.Helper()
	doc, err := config.Parse([]byte(enginetest.RouteJSON("null", raw)))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := doc.Config()
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Branches[0].Transform.Steps
}

func testLibrary(t *testing.T) *countingLibrary {
	lib := &countingLibrary{InMemoryLibrary: configtest.NewInMemoryLibrary()}
	lib.PutTransform("clean", 1, steps(t, `[{"id":"r","kind":"rename","from":"value.a","to":"value.b"}]`))
	lib.PutTransform("clean", 2, steps(t, `[{"id":"r","kind":"rename","from":"value.a","to":"value.c"},
		{"id":"strict","kind":"remove","fields":["value.gone"],"strict":true}]`))
	lib.PutTransform("loop", 1, steps(t, `[{"id":"self","kind":"library","ref":"loop","version":1}]`))
	lib.PutTransform("outer", 1, steps(t, `[{"id":"in","kind":"library","ref":"clean","version":1}]`))
	lib.PutTransform("broken", 1, steps(t, `[{"id":"ok","kind":"rename","from":"value.a","to":"value.b"},{"id":"bad","kind":"rename","from":"value..a","to":"value.b"}]`))
	lib.PutLookupTable("cc", 1, config.LookupRows{{Key: "DE", Value: "Deutschland"}})
	lib.PutLookupTable("cc", 2, config.LookupRows{{Key: "DE", Value: "Germany"}})
	return lib
}

func compileWith(t *testing.T, lib config.LibraryResolver, stepsJSON string) (engine.Pipeline, *engine.CompileError) {
	t.Helper()
	doc, err := config.Parse([]byte(enginetest.RouteJSON("null", stepsJSON)))
	if err != nil {
		t.Fatal(err)
	}
	opts := enginetest.GoldenOptions()
	opts.Library = lib
	p, err := compile.NewDefaultCompiler().Compile(ctx, doc, opts)
	if err != nil {
		var ce *engine.CompileError
		if !errors.As(err, &ce) {
			t.Fatal(err)
		}
		return nil, ce
	}
	t.Cleanup(func() { _ = p.Close(ctx) })
	return p, nil
}

func TestLibraryStepPinsVersionAndSplicesTrace(t *testing.T) {
	for v, field := range map[string]string{"1": "b", "2": "c"} {
		p, ce := compileWith(t, testLibrary(t), `[{"id":"lib","kind":"library","ref":"clean","version":`+v+`}]`)
		if ce != nil {
			t.Fatalf("v%s: %+v", v, ce.Issues)
		}
		tr := trace.NewRecordingTracer(0)
		outs, err := p.Process(ctx, enginetest.MustDecode(t, p, `{"a":1,"gone":0}`), tr)
		if err != nil || len(outs) != 1 || outs[0].Value.(map[string]any)[field] != int64(1) {
			t.Fatalf("v%s: %v %+v", v, err, outs)
		}
		got := tr.Trace().Branches[0].Steps
		if got[0].ID != "lib/clean@"+v+"/r" {
			t.Fatalf("v%s trace ids: %+v", v, got)
		}
	}
}

func TestLibraryRuntimeErrorNamesNestedStep(t *testing.T) {
	p, ce := compileWith(t, testLibrary(t), `[{"kind":"set","field":"value.x","source":{"from":"constant","value":1}},{"id":"lib","kind":"library","ref":"clean","version":2}]`)
	if ce != nil {
		t.Fatal(ce.Issues)
	}
	_, err := p.Process(ctx, enginetest.MustDecode(t, p, `{"a":1}`), nil)
	var pe *engine.ProcessError
	if !errors.As(err, &pe) || pe.StepID != "lib/clean@2/strict" || pe.Path != "/branches/0/transform/steps/1" {
		t.Fatalf("%#v", err)
	}
}

func TestLibraryCompileIssues(t *testing.T) {
	cases := []struct {
		name, steps string
		lib         config.LibraryResolver
		path        config.Pointer
		code        config.IssueCode
		message     string
	}{
		{"deleted version", `[{"kind":"library","ref":"clean","version":9}]`, testLibrary(t), "/branches/0/transform/steps/0/ref", config.IssueLibraryRefMissing, "clean@9 does not exist"},
		{"unknown ref", `[{"kind":"library","ref":"nope","version":1}]`, testLibrary(t), "/branches/0/transform/steps/0/ref", config.IssueLibraryRefMissing, "nope@1"},
		{"no resolver", `[{"kind":"library","ref":"clean","version":1}]`, nil, "/branches/0/transform/steps/0/ref", config.IssueLibraryRefMissing, "no library"},
		{"store down", `[{"kind":"library","ref":"clean","version":1}]`, failingLibrary{}, "/branches/0/transform/steps/0/ref", config.IssueLibraryRefMissing, "connection refused"},
		{"cycle", `[{"kind":"library","ref":"loop","version":1}]`, testLibrary(t), "/branches/0/transform/steps/0", config.IssueLibraryRefMissing, "includes itself"},
		{"nested issue", `[{"kind":"library","ref":"broken","version":1}]`, testLibrary(t), "/branches/0/transform/steps/0", config.IssuePathInvalid, "step /1/from"},
		{"missing table", `[{"kind":"lookup","field":"value.c","ref":"cc","version":7}]`, testLibrary(t), "/branches/0/transform/steps/0/ref", config.IssueLibraryRefMissing, "lookup table cc@7"},
	}
	for _, tc := range cases {
		_, ce := compileWith(t, tc.lib, tc.steps)
		if ce == nil || len(ce.Issues) != 1 {
			t.Fatalf("%s: want one issue, got %+v", tc.name, ce)
		}
		is := ce.Issues[0]
		if is.Path != tc.path || is.Code != tc.code || !strings.Contains(is.Message, tc.message) {
			t.Errorf("%s: %+v", tc.name, is)
		}
	}
}

func TestNestedLibraryAndInsideIfElse(t *testing.T) {
	p, ce := compileWith(t, testLibrary(t), `[{"id":"o","kind":"library","ref":"outer","version":1},
		{"id":"ie","kind":"if_else","condition":{"type":"cond","field":"value.b","op":"exists"},
		 "thenSteps":[{"id":"l2","kind":"library","ref":"clean","version":2}]}]`)
	if ce != nil {
		t.Fatal(ce.Issues)
	}
	tr := trace.NewRecordingTracer(0)
	outs, err := p.Process(ctx, enginetest.MustDecode(t, p, `{"a":1,"gone":true}`), tr)
	want := map[string]any{"b": int64(1)}
	if err != nil || len(outs) != 1 || !reflect.DeepEqual(outs[0].Value, want) {
		t.Fatalf("%v %+v", err, outs)
	}
	if id := tr.Trace().Branches[0].Steps[0].ID; id != "o/outer@1/in/clean@1/r" {
		t.Fatalf("nested id %s", id)
	}
}

func TestLibraryLoadsEachPinOnce(t *testing.T) {
	lib := testLibrary(t)
	_, ce := compileWith(t, lib, `[{"id":"a","kind":"library","ref":"clean","version":1},{"id":"b","kind":"library","ref":"clean","version":1},
		{"id":"l1","kind":"lookup","field":"value.c","to":"value.x","ref":"cc","version":2},{"id":"l2","kind":"lookup","field":"value.c","to":"value.y","ref":"cc","version":2}]`)
	if ce != nil {
		t.Fatal(ce.Issues)
	}
	if lib.transforms != 1 || lib.tables != 1 {
		t.Fatalf("loads: transforms=%d tables=%d", lib.transforms, lib.tables)
	}
}

func TestLibraryLookupTableNames(t *testing.T) {
	p, ce := compileWith(t, testLibrary(t), `[{"id":"one","kind":"lookup","field":"value.c","to":"value.v1","ref":"cc","version":1},
		{"id":"two","kind":"lookup","field":"value.c","to":"value.v2","ref":"cc","version":2},
		{"kind":"set","field":"value.f","source":{"from":"formula","expr":"concat(lookup(\"cc@1\", value.c), \"/\", lookup(\"one\", value.c), \"/\", lookup(\"cc\", value.c, \"ambiguous\"))"}}]`)
	if ce != nil {
		t.Fatal(ce.Issues)
	}
	outs, err := p.Process(ctx, enginetest.MustDecode(t, p, `{"c":"DE"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	v := outs[0].Value.(map[string]any)
	if v["v1"] != "Deutschland" || v["v2"] != "Germany" || v["f"] != "Deutschland/Deutschland/ambiguous" {
		t.Fatalf("%+v", v)
	}
}

// TestReplayIsDeterministic backs PE-103: replay and exactly-once need no engine change
// because a compiled pipeline is a pure function of (record, frozen clock, pinned library).
func TestReplayIsDeterministic(t *testing.T) {
	route := `[{"id":"lib","kind":"library","ref":"clean","version":1},
		{"id":"cc","kind":"lookup","field":"value.c","to":"value.country","ref":"cc","version":2},
		{"kind":"set","field":"value.at","source":{"from":"formula","expr":"formatDate(now(), \"rfc3339\")"}},
		{"kind":"set","field":"value.offset","source":{"from":"metadata","name":"offset"}}]`
	rec := engine.RawRecord{Value: []byte(`{"a":1,"c":"DE"}`), Topic: "in", Offset: 42, Timestamp: enginetest.GoldenNow}
	var first []engine.Out
	for run := 0; run < 2; run++ {
		p, ce := compileWith(t, testLibrary(t), route)
		if ce != nil {
			t.Fatal(ce.Issues)
		}
		for attempt := 0; attempt < 2; attempt++ {
			ev, err := p.Decode(ctx, rec)
			if err != nil {
				t.Fatal(err)
			}
			outs, err := p.Process(ctx, ev, nil)
			if err != nil {
				t.Fatal(err)
			}
			if first == nil {
				first = outs
				continue
			}
			if !reflect.DeepEqual(first, outs) {
				t.Fatalf("run %d attempt %d differs:\n%+v\n%+v", run, attempt, first, outs)
			}
		}
	}
}
