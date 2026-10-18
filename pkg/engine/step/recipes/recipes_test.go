package recipes_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/enginetest"
	"github.com/eventrouter/router/pkg/engine/step"
)

type recipeCase struct {
	name    string
	steps   string
	in      string
	want    string
	wantKey any
	wantErr step.StepErrorReason
	dropped bool
}

func run(t *testing.T, c recipeCase) {
	t.Helper()
	p := enginetest.MustCompile(t, enginetest.RouteJSON("null", c.steps))
	ev := enginetest.MustDecode(t, p, c.in)
	ev.Headers = []engine.Header{{Key: "h", Value: []byte("hv")}}
	outs, err := p.Process(context.Background(), ev, nil)
	if c.wantErr != "" {
		var se *step.StepError
		var pe *engine.ProcessError
		if !errors.As(err, &se) || se.Reason != c.wantErr || !errors.As(err, &pe) || pe.Type != engine.ErrStep {
			t.Fatalf("%s: want %s, got %v", c.name, c.wantErr, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s: %v", c.name, err)
	}
	if c.dropped {
		if len(outs) != 0 {
			t.Fatalf("%s: want dropped, got %+v", c.name, outs)
		}
		return
	}
	var want any
	_ = json.Unmarshal([]byte(c.want), &want)
	got, _ := json.Marshal(outs[0].Value)
	var gotAny any
	_ = json.Unmarshal(got, &gotAny)
	if !reflect.DeepEqual(gotAny, want) {
		t.Fatalf("%s: got %s want %s", c.name, got, c.want)
	}
	if c.wantKey != nil && !reflect.DeepEqual(outs[0].Key, c.wantKey) {
		t.Fatalf("%s: key %#v", c.name, outs[0].Key)
	}
}

func TestRecipes(t *testing.T) {
	cases := []recipeCase{
		{name: "rename", steps: `[{"kind":"rename","from":"value.a","to":"value.b"}]`, in: `{"a":1}`, want: `{"b":1}`},
		{name: "rename nested", steps: `[{"kind":"rename","from":"value.o.a","to":"value.n.x.y"}]`, in: `{"o":{"a":1,"k":2}}`, want: `{"o":{"k":2},"n":{"x":{"y":1}}}`},
		{name: "rename missing lenient", steps: `[{"kind":"rename","from":"value.z","to":"value.b"}]`, in: `{"a":1}`, want: `{"a":1}`},
		{name: "rename missing strict", steps: `[{"kind":"rename","from":"value.z","to":"value.b","strict":true}]`, in: `{"a":1}`, wantErr: step.ReasonFieldMissing},
		{name: "rename into scalar", steps: `[{"kind":"rename","from":"value.a","to":"value.b.c"}]`, in: `{"a":1,"b":2}`, wantErr: step.ReasonWriteFailed},
		{name: "copy deep", steps: `[{"kind":"copy","from":"value.o","to":"value.c"},{"kind":"set","field":"value.o.x","source":{"from":"constant","value":9}}]`, in: `{"o":{"x":1}}`, want: `{"o":{"x":9},"c":{"x":1}}`},
		{name: "move to header", steps: `[{"kind":"move","from":"value.t","to":"headers.t"},{"kind":"set","field":"value.h","source":{"from":"field","path":"headers.t"}}]`, in: `{"t":"x"}`, want: `{"h":"x"}`},
		{name: "move array element", steps: `[{"kind":"move","from":"value.a[0]","to":"value.first"}]`, in: `{"a":[1,2]}`, want: `{"a":[2],"first":1}`},
		{name: "remove many", steps: `[{"kind":"remove","fields":["value.a","value.b.c","value.z"]}]`, in: `{"a":1,"b":{"c":2,"d":3}}`, want: `{"b":{"d":3}}`},
		{name: "remove strict", steps: `[{"kind":"remove","fields":["value.z"],"strict":true}]`, in: `{}`, wantErr: step.ReasonFieldMissing},
		{name: "keep_only", steps: `[{"kind":"keep_only","fields":["value.a","value.b.c","value.l"]}]`, in: `{"a":1,"b":{"c":2,"d":3},"x":4,"l":[5,6]}`, want: `{"a":1,"b":{"c":2},"l":[5,6]}`},
		{name: "keep_only strict", steps: `[{"kind":"keep_only","fields":["value.q"],"strict":true}]`, in: `{"a":1}`, wantErr: step.ReasonFieldMissing},
		{name: "set constant object", steps: `[{"kind":"set","field":"value.m","source":{"from":"constant","value":{"k":[1,"a"]}}}]`, in: `{}`, want: `{"m":{"k":[1,"a"]}}`},
		{name: "set metadata", steps: `[{"kind":"set","field":"value.m","source":{"from":"metadata","name":"timestamp"}},{"kind":"set","field":"value.p","source":{"from":"metadata","name":"partition"}},{"kind":"set","field":"value.r","source":{"from":"metadata","name":"route_name"}}]`, in: `{}`, want: `{"m":"2026-01-01T00:00:00Z","p":0,"r":""}`},
		{name: "set from missing strict", steps: `[{"kind":"set","field":"value.x","strict":true,"source":{"from":"field","path":"value.nope"}}]`, in: `{}`, wantErr: step.ReasonFieldMissing},
		{name: "set on null value", steps: `[{"kind":"set","field":"value.x","source":{"from":"constant","value":1}}]`, in: `null`, want: `{"x":1}`},
		{name: "default_if_missing present", steps: `[{"kind":"default_if_missing","field":"value.c","value":"EUR"}]`, in: `{"c":"USD"}`, want: `{"c":"USD"}`},
		{name: "default_if_missing null", steps: `[{"kind":"default_if_missing","field":"value.c","value":"EUR"}]`, in: `{"c":null}`, want: `{"c":"EUR"}`},
		{name: "set_key", steps: `[{"kind":"set_key","source":{"from":"field","path":"value.id"}}]`, in: `{"id":42}`, want: `{"id":42}`, wantKey: int64(42)},
		{name: "set_key strict missing", steps: `[{"kind":"set_key","strict":true,"source":{"from":"field","path":"value.id"}}]`, in: `{}`, wantErr: step.ReasonFieldMissing},
		{name: "mask styles", steps: `[{"kind":"mask","field":"value.e","style":"email"},{"kind":"mask","field":"value.c","style":"last4"},{"kind":"mask","field":"value.n","style":"full"}]`, in: `{"e":"ab@c.de","c":"12345678","n":12345}`, want: `{"e":"a*@c.de","c":"****5678","n":"*****"}`},
		{name: "mask null stays null", steps: `[{"kind":"mask","field":"value.e","style":"email"}]`, in: `{"e":null}`, want: `{"e":null}`},
		{name: "hash", steps: `[{"kind":"hash","field":"value.e"}]`, in: `{"e":"x"}`, want: `{"e":"2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881"}`},
		{name: "hash missing strict", steps: `[{"kind":"hash","field":"value.e","strict":true}]`, in: `{}`, wantErr: step.ReasonFieldMissing},
		{name: "convert number", steps: `[{"kind":"convert_type","field":"value.a","targetType":"number"},{"kind":"convert_type","field":"value.b","targetType":"number"}]`, in: `{"a":"1.5","b":"2"}`, want: `{"a":1.5,"b":2}`},
		{name: "convert integer fail error", steps: `[{"kind":"convert_type","field":"value.a","targetType":"integer"}]`, in: `{"a":"1.5"}`, wantErr: step.ReasonConvertFailed},
		{name: "convert onFailure null", steps: `[{"kind":"convert_type","field":"value.a","targetType":"boolean","onFailure":"null"}]`, in: `{"a":[1]}`, want: `{"a":null}`},
		{name: "convert onFailure keep", steps: `[{"kind":"convert_type","field":"value.a","targetType":"number","onFailure":"keep"}]`, in: `{"a":"x"}`, want: `{"a":"x"}`},
		{name: "convert string", steps: `[{"kind":"convert_type","field":"value.a","targetType":"string"},{"kind":"convert_type","field":"value.b","targetType":"string"}]`, in: `{"a":false,"b":[1]}`, want: `{"a":"false","b":"[1]"}`},
		{name: "convert boolean", steps: `[{"kind":"convert_type","field":"value.a","targetType":"boolean"},{"kind":"convert_type","field":"value.b","targetType":"boolean"}]`, in: `{"a":" TRUE ","b":0}`, want: `{"a":true,"b":false}`},
		{name: "drop_if true", steps: `[{"kind":"drop_if","condition":{"type":"cond","field":"value.t","op":"eq","value":true}}]`, in: `{"t":true}`, dropped: true},
		{name: "drop_if false", steps: `[{"kind":"drop_if","condition":{"type":"cond","field":"value.t","op":"eq","value":true}}]`, in: `{"t":false}`, want: `{"t":false}`},
		{name: "disabled skipped", steps: `[{"kind":"remove","fields":["value.a"],"enabled":false}]`, in: `{"a":1}`, want: `{"a":1}`},
		{name: "array of strings untouched", steps: `[{"kind":"rename","from":"value.a[1]","to":"value.b"}]`, in: `{"a":["x","y"]}`, want: `{"a":["x"],"b":"y"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { run(t, c) })
	}
}

func TestRecipeCompileIssues(t *testing.T) {
	cases := map[string]config.Pointer{
		`[{"kind":"rename","from":"value..a","to":"value.b"}]`:                                        "/branches/0/transform/steps/0/from",
		`[{"kind":"keep_only","fields":["value.a","key.b"]}]`:                                         "/branches/0/transform/steps/0/fields/1",
		`[{"kind":"set","field":"value.a","source":{"from":"formula","expr":"x"}}]`:                   "/branches/0/transform/steps/0/source/expr",
		`[{"kind":"set","field":"value.a","source":{"from":"field","path":"zz.a"}}]`:                  "/branches/0/transform/steps/0/source/path",
		`[{"kind":"drop_if","condition":{"type":"cond","field":"value.a","op":"regex","value":"["}}]`: "/branches/0/transform/steps/0/condition/value",
		`[{"kind":"js","code":"unknown"}]`:                                                            "/branches/0/transform/steps/0/code",
		`[{"kind":"library","ref":"x","version":1}]`:                                                  "/branches/0/transform/steps/0/ref",
	}
	for steps, want := range cases {
		doc, _ := config.Parse([]byte(enginetest.RouteJSON("null", steps)))
		_, err := compile.NewDefaultCompiler().Compile(context.Background(), doc, enginetest.GoldenOptions())
		var ce *engine.CompileError
		if !errors.As(err, &ce) || len(ce.Issues) != 1 || ce.Issues[0].Path != want {
			t.Errorf("%s: want issue at %s, got %v", steps, want, err)
		}
	}
}

func TestStepIDs(t *testing.T) {
	if step.IDFor(config.Step{}, 3) != "step-3" {
		t.Fatal("generated id")
	}
	id := "x"
	if step.IDFor(config.Step{ID: &id}, 3) != "x" {
		t.Fatal("explicit id")
	}
	kinds := []string{}
	for _, c := range compile.DefaultRegistries().Steps.All() {
		kinds = append(kinds, string(c.Kind()))
	}
	want := "rename,remove,keep_only,copy,move,set,default_if_missing,set_key,mask,hash,convert_type,drop_if,js,library," +
		"flatten,redact_pattern,format_date,round,string_case,lookup,if_else,split_array,wrap,unwrap," +
		"header_add,header_remove,header_rename,header_copy_to_payload,header_copy_from_payload,map"
	if strings.Join(kinds, ",") != want {
		t.Fatalf("kinds %v", kinds)
	}
}

func FuzzRecipesNoPanic(f *testing.F) {
	for _, s := range []string{`{"a":1,"b":{"c":[1,2,3]},"e":"x@y.z"}`, `[]`, `"s"`, `null`, `{"b":"str","l":[[[]]]}`, `{"a":{"a":{"a":null}}}`} {
		f.Add([]byte(s))
	}
	steps := `[
	 {"kind":"rename","from":"value.a","to":"value.b.c"},
	 {"kind":"copy","from":"value.b","to":"value.l[0]"},
	 {"kind":"move","from":"value.l[1]","to":"key.k"},
	 {"kind":"remove","fields":["value.b.c[2]","value.e"]},
	 {"kind":"set","field":"value.a.a.a","source":{"from":"field","path":"value.e"}},
	 {"kind":"default_if_missing","field":"value.d","value":[1]},
	 {"kind":"mask","field":"value.e","style":"email"},
	 {"kind":"hash","field":"value.b"},
	 {"kind":"convert_type","field":"value.d","targetType":"integer","onFailure":"null"},
	 {"kind":"set_key","source":{"from":"field","path":"value.a"}},
	 {"kind":"drop_if","condition":{"type":"cond","field":"value.zz","op":"exists"}},
	 {"kind":"keep_only","fields":["value.a","value.b","value.d","value.l"]},
	 {"kind":"flatten","field":"value","separator":"_"},
	 {"kind":"redact_pattern","field":"value.a","preset":"credit_card"},
	 {"kind":"format_date","field":"value.d","layout":"date","to":"value.day"},
	 {"kind":"round","field":"value.b","decimals":1},
	 {"kind":"string_case","field":"value.a","case":"title"},
	 {"kind":"lookup","field":"value.a","to":"value.c","rows":[{"key":"1","value":"one"}],"defaultValue":null},
	 {"kind":"if_else","condition":{"type":"cond","field":"value.a","op":"exists"},"thenSteps":[{"kind":"wrap","field":"value.w"}],"elseSteps":[{"kind":"unwrap","field":"value.l"}]},
	 {"kind":"split_array","field":"value.w.l"},
	 {"kind":"header_add","header":"h","source":{"from":"formula","expr":"concat(value.a, len(value), 1/len(value))"}},
	 {"kind":"header_rename","header":"h","newHeader":"h2"},
	 {"kind":"header_copy_to_payload","header":"h2","field":"value.h"},
	 {"kind":"header_copy_from_payload","field":"value.w","header":"w"},
	 {"kind":"header_remove","header":"h2"},
	 {"kind":"map","start":"copy_input","target":{"fields":{"m":{"source":{"from":"field","path":"value.w"}},"w":{"remove":true}}}}
	]`
	p := enginetest.MustCompile(f, enginetest.RouteJSON("null", steps))
	f.Fuzz(func(t *testing.T, raw []byte) {
		ev, err := p.Decode(context.Background(), engine.RawRecord{Value: raw})
		if err != nil {
			return
		}
		outs, err := p.Process(context.Background(), ev, nil)
		if err != nil {
			var pe *engine.ProcessError
			if !errors.As(err, &pe) {
				t.Fatalf("untyped error %v", err)
			}
			return
		}
		for _, o := range outs {
			_, _ = p.Encode(context.Background(), o)
		}
	})
}
