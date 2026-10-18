package export_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/assist/describe"
	"github.com/eventrouter/router/internal/assist/export"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/enginetest"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/engines"
	"github.com/eventrouter/router/pkg/sandbox/sandboxtest"
)

const team = "t1"

func exporter() *export.JsExporter {
	return export.NewJsExporter(export.DefaultJsWriterManager(), describe.DefaultDescriberManager())
}

func decodeSteps(t *testing.T, raw string) []config.Step {
	t.Helper()
	var steps []config.Step
	if err := json.Unmarshal([]byte(raw), &steps); err != nil {
		t.Fatal(err)
	}
	return steps
}

func TestExportRejectsUnsupportedSteps(t *testing.T) {
	steps := decodeSteps(t, `[{"kind":"rename","from":"value.a","to":"value.b"},
		{"kind":"library","ref":"x","version":1},
		{"kind":"set","field":"value.c","source":{"from":"formula","expr":"a+1"}},
		{"kind":"remove","fields":["value.a","headers.x"]},
		{"kind":"hash","field":"value.a","strict":true}]`)
	_, issues := exporter().Export(config.Pointer("/steps"), steps)
	want := []config.Pointer{"/steps/1/kind", "/steps/2/source", "/steps/3/fields/1", "/steps/4"}
	if len(issues) != len(want) {
		t.Fatalf("issues %+v", issues)
	}
	for i, p := range want {
		if issues[i].Path != p || issues[i].Code != config.IssueStepUnsupported {
			t.Errorf("issue %d = %+v, want path %s", i, issues[i], p)
		}
	}
}

func TestExportSkipsDisabledAndNeedsOneStep(t *testing.T) {
	steps := decodeSteps(t, `[{"kind":"hash","field":"value.a","enabled":false}]`)
	if _, issues := exporter().Export(config.Pointer("/steps"), steps); len(issues) != 1 {
		t.Fatalf("want one 'nothing to export' issue, got %+v", issues)
	}
}

// parityCase runs the recipes through pkg/engine and the exported JS through the sandbox (TS-121).
type parityCase struct {
	name, steps, value string
}

var parityCases = []parityCase{
	{"rename", `[{"kind":"rename","from":"value.cust_id","to":"value.customer.id"}]`, `{"cust_id":"C1","x":1}`},
	{"move missing", `[{"kind":"move","from":"value.nope","to":"value.y"}]`, `{"x":1}`},
	{"copy", `[{"kind":"copy","from":"value.items[0]","to":"value.first"}]`, `{"items":[{"p":1},{"p":2}]}`},
	{"remove", `[{"kind":"remove","fields":["value.a","value.b.c","value.zz"]}]`, `{"a":1,"b":{"c":2,"d":3}}`},
	{"keep_only", `[{"kind":"keep_only","fields":["value.a","value.b.c"]}]`, `{"a":1,"b":{"c":2,"d":3},"e":4}`},
	{"set constant", `[{"kind":"set","field":"value.meta.v","source":{"from":"constant","value":{"n":2}}}]`, `{"a":1}`},
	{"set field", `[{"kind":"set","field":"value.copy","source":{"from":"field","path":"value.a"}}]`, `{"a":[1,2]}`},
	{"default", `[{"kind":"default_if_missing","field":"value.c","value":"DE"},{"kind":"default_if_missing","field":"value.a","value":0}]`, `{"a":null}`},
	{"set_key", `[{"kind":"set_key","source":{"from":"field","path":"value.id"}}]`, `{"id":"k-1"}`},
	{"mask", `[{"kind":"mask","field":"value.email","style":"email"},{"kind":"mask","field":"value.card","style":"last4"},{"kind":"mask","field":"value.n","style":"full"}]`,
		`{"email":"jane.doe@example.com","card":"4111111111111111","n":null}`},
	{"hash", `[{"kind":"hash","field":"value.id"},{"kind":"hash","field":"value.none"}]`, `{"id":"abc"}`},
	{"chain", `[{"kind":"rename","from":"value.a","to":"value.b"},{"kind":"mask","field":"value.b","style":"full"},{"kind":"remove","fields":["value.c"]}]`,
		`{"a":"secret","c":3}`},
}

func TestExportedJsMatchesRecipes(t *testing.T) {
	m := newSandbox(t)
	for _, tc := range parityCases {
		t.Run(tc.name, func(t *testing.T) {
			step, issues := exporter().Export(config.Pointer("/steps"), decodeSteps(t, tc.steps))
			if len(issues) > 0 {
				t.Fatalf("export issues %+v", issues)
			}
			wantKey, wantValue := runRecipes(t, tc.steps, tc.value)
			gotKey, gotValue := runJS(t, m, *step.Code, tc.value)
			if !reflect.DeepEqual(norm(gotValue), norm(wantValue)) || fmt.Sprint(gotKey) != fmt.Sprint(wantKey) {
				t.Errorf("parity:\n js     key=%v value=%v\n recipe key=%v value=%v\ncode:\n%s", gotKey, gotValue, wantKey, wantValue, *step.Code)
			}
		})
	}
}

func newSandbox(t *testing.T) sandbox.Manager {
	t.Helper()
	m, err := sandbox.NewManager(context.Background(), sandbox.DefaultConfig(), sandbox.ManagerDeps{
		Engines: engines.Registry(), Salts: sandboxtest.Salts{}, Binder: enginebind.Binder{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	return m
}

func runRecipes(t *testing.T, steps, value string) (any, any) {
	t.Helper()
	doc, err := config.Parse([]byte(enginetest.RouteJSON("null", steps)))
	if err != nil {
		t.Fatal(err)
	}
	salt, _ := sandboxtest.Salts{}.Salt(context.Background(), team)
	opts := engine.CompileOptions{Helpers: ctxhelpers.Set{Hash: ctxhelpers.NewSaltedHasher(ctxhelpers.StaticSaltProvider{Value: salt}, team)}.WithDefaults()}
	p, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, opts)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	t.Cleanup(func() { _ = p.Close(context.Background()) })
	outs, err := p.Process(context.Background(), enginetest.MustDecode(t, p, value), nil)
	if err != nil || len(outs) != 1 {
		t.Fatalf("process: %v %d", err, len(outs))
	}
	return outs[0].Key, outs[0].Value
}

func runJS(t *testing.T, m sandbox.Manager, code, value string) (any, any) {
	t.Helper()
	mod, _ := sandboxtest.LoadOne(t, m, sandbox.EntryTransform, code)
	var v any
	if err := json.Unmarshal([]byte(value), &v); err != nil {
		t.Fatal(err)
	}
	res, err := mod.Call(context.Background(), sandbox.Invocation{Event: enginebind.SnapshotCodec{}.ToGuest(map[string]any{"value": v}), Now: time.Unix(1, 0)})
	if err != nil || len(res.Outputs) != 1 {
		t.Fatalf("js call: %v (%d outputs)\n%s", err, len(res.Outputs), code)
	}
	return res.Outputs[0].Key, res.Outputs[0].Value
}

// norm compares numbers by value regardless of Go numeric type.
func norm(v any) any {
	raw, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(raw, &out)
	return out
}

func TestExportedSourceShape(t *testing.T) {
	step, _ := exporter().Export(config.Pointer("/steps"), decodeSteps(t, `[{"kind":"rename","from":"value.a","to":"value.b"}]`))
	if step.Kind != config.StepKindJS || !strings.Contains(*step.Code, "function transform(event, ctx)") ||
		!strings.Contains(*step.Code, "// 1. Rename a → b") {
		t.Fatalf("unexpected code:\n%s", *step.Code)
	}
}
