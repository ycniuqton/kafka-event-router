package recipes_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/enginetest"
	"github.com/eventrouter/router/pkg/engine/step"
)

// multiCase checks every output value (as a JSON array) and optionally the headers of the first.
type multiCase struct {
	name        string
	steps       string
	in          string
	want        string
	wantHeaders map[string]string
	wantErr     step.StepErrorReason
}

func runMulti(t *testing.T, c multiCase) {
	t.Helper()
	p := enginetest.MustCompile(t, enginetest.RouteJSON("null", c.steps))
	ev := enginetest.MustDecode(t, p, c.in)
	ev.Headers = []engine.Header{{Key: "h", Value: []byte("hv")}, {Key: "trace.id", Value: []byte("t-1")}}
	outs, err := p.Process(context.Background(), ev, nil)
	if c.wantErr != "" {
		var se *step.StepError
		if !errors.As(err, &se) || se.Reason != c.wantErr {
			t.Fatalf("%s: want %s, got %v", c.name, c.wantErr, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s: %v", c.name, err)
	}
	vals := make([]any, 0, len(outs))
	for _, o := range outs {
		vals = append(vals, o.Value)
	}
	got, _ := json.Marshal(vals)
	var gotAny, wantAny any
	_ = json.Unmarshal(got, &gotAny)
	_ = json.Unmarshal([]byte(c.want), &wantAny)
	if !reflect.DeepEqual(gotAny, wantAny) {
		t.Fatalf("%s: got %s want %s", c.name, got, c.want)
	}
	if c.wantHeaders != nil {
		hs := map[string]string{}
		for _, h := range outs[0].Headers {
			hs[h.Key] = string(h.Value)
		}
		if !reflect.DeepEqual(hs, c.wantHeaders) {
			t.Fatalf("%s: headers %v want %v", c.name, hs, c.wantHeaders)
		}
	}
}

func TestRecipesBatchD(t *testing.T) {
	cases := []multiCase{
		{name: "flatten", steps: `[{"kind":"flatten","field":"value"}]`, in: `{"a":{"b":{"c":1},"l":[{"x":1}],"e":{}},"z":2}`, want: `[{"a.b.c":1,"a.l":[{"x":1}],"a.e":{},"z":2}]`},
		{name: "flatten separator to", steps: `[{"kind":"flatten","field":"value.o","separator":"_","to":"value.f"}]`, in: `{"o":{"a":{"b":1}}}`, want: `[{"o":{"a":{"b":1}},"f":{"a_b":1}}]`},
		{name: "flatten scalar untouched", steps: `[{"kind":"flatten","field":"value.s"}]`, in: `{"s":"x"}`, want: `[{"s":"x"}]`},
		{name: "redact card", steps: `[{"kind":"redact_pattern","field":"value.t","preset":"credit_card"}]`, in: `{"t":"card 4111 1111 1111 1111 ok"}`, want: `[{"t":"card [REDACTED] ok"}]`},
		{name: "redact phone custom replacement", steps: `[{"kind":"redact_pattern","field":"value.t","preset":"phone","replacement":"<tel>"}]`, in: `{"t":"call +49 30 1234567 now"}`, want: `[{"t":"call <tel> now"}]`},
		{name: "redact email", steps: `[{"kind":"redact_pattern","field":"value.t","preset":"email"}]`, in: `{"t":"mail a.b@x.io"}`, want: `[{"t":"mail [REDACTED]"}]`},
		{name: "redact iban", steps: `[{"kind":"redact_pattern","field":"value.t","preset":"iban"}]`, in: `{"t":"DE89 3704 0044 0532 0130 00"}`, want: `[{"t":"[REDACTED]"}]`},
		{name: "redact custom", steps: `[{"kind":"redact_pattern","field":"value.t","pattern":"\\d+","replacement":"#"}]`, in: `{"t":"a1b22"}`, want: `[{"t":"a#b#"}]`},
		{name: "redact non text", steps: `[{"kind":"redact_pattern","field":"value.t","preset":"email"}]`, in: `{"t":5}`, want: `[{"t":5}]`},
		{name: "format_date tz", steps: `[{"kind":"format_date","field":"value.t","layout":"datetime","timezone":"America/New_York"}]`, in: `{"t":"2026-01-01T12:00:00Z"}`, want: `[{"t":"2026-01-01 07:00:00"}]`},
		{name: "format_date epoch to", steps: `[{"kind":"format_date","field":"value.t","layout":"rfc3339_ms","to":"value.iso"}]`, in: `{"t":1700000000123}`, want: `[{"t":1700000000123,"iso":"2023-11-14T22:13:20.123Z"}]`},
		{name: "format_date bad", steps: `[{"kind":"format_date","field":"value.t","layout":"date"}]`, in: `{"t":"yesterday"}`, wantErr: step.ReasonConvertFailed},
		{name: "format_date missing strict", steps: `[{"kind":"format_date","field":"value.t","layout":"date","strict":true}]`, in: `{}`, wantErr: step.ReasonFieldMissing},
		{name: "round", steps: `[{"kind":"round","field":"value.a","decimals":2},{"kind":"round","field":"value.b","decimals":0},{"kind":"round","field":"value.c","decimals":1}]`, in: `{"a":1.005,"b":2.5,"c":"3.14"}`, want: `[{"a":1,"b":3,"c":3.1}]`},
		{name: "round not number", steps: `[{"kind":"round","field":"value.a","decimals":1}]`, in: `{"a":"x"}`, wantErr: step.ReasonConvertFailed},
		{name: "round null", steps: `[{"kind":"round","field":"value.a","decimals":1}]`, in: `{"a":null}`, want: `[{"a":null}]`},
		{name: "string_case", steps: `[{"kind":"string_case","field":"value.a","case":"upper"},{"kind":"string_case","field":"value.b","case":"lower"},{"kind":"string_case","field":"value.c","case":"title"},{"kind":"string_case","field":"value.d","case":"trim"},{"kind":"string_case","field":"value.e","case":"upper"}]`,
			in: `{"a":"abc","b":"ABC","c":"jean-luc picard","d":"  x ","e":1}`, want: `[{"a":"ABC","b":"abc","c":"Jean-Luc Picard","d":"x","e":1}]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { runMulti(t, c) })
	}
}

func TestRecipesBatchE(t *testing.T) {
	rows := `[{"key":"DE","value":"Germany"},{"key":"1","value":{"n":1}}]`
	cases := []multiCase{
		{name: "lookup hit", steps: `[{"id":"cc","kind":"lookup","field":"value.cc","to":"value.country","rows":` + rows + `}]`, in: `{"cc":"DE"}`, want: `[{"cc":"DE","country":"Germany"}]`},
		{name: "lookup number key in place", steps: `[{"kind":"lookup","field":"value.k","rows":` + rows + `}]`, in: `{"k":1}`, want: `[{"k":{"n":1}}]`},
		{name: "lookup miss default", steps: `[{"kind":"lookup","field":"value.cc","to":"value.country","rows":` + rows + `,"defaultValue":"unknown"}]`, in: `{"cc":"FR"}`, want: `[{"cc":"FR","country":"unknown"}]`},
		{name: "lookup miss no default", steps: `[{"kind":"lookup","field":"value.cc","to":"value.country","rows":` + rows + `}]`, in: `{"cc":"FR"}`, want: `[{"cc":"FR"}]`},
		{name: "lookup table via formula", steps: `[{"id":"countries","kind":"lookup","field":"value.x","rows":` + rows + `},{"kind":"set","field":"value.c","source":{"from":"formula","expr":"lookup(\"countries\", value.cc, \"?\")"}}]`, in: `{"cc":"DE"}`, want: `[{"cc":"DE","c":"Germany"}]`},
		{name: "if_else then", steps: `[{"kind":"if_else","condition":{"type":"cond","field":"value.n","op":"gt","value":10},"thenSteps":[{"kind":"set","field":"value.big","source":{"from":"constant","value":true}}],"elseSteps":[{"kind":"remove","fields":["value.n"]}]}]`, in: `{"n":11}`, want: `[{"n":11,"big":true}]`},
		{name: "if_else else", steps: `[{"kind":"if_else","condition":{"type":"cond","field":"value.n","op":"gt","value":10},"thenSteps":[{"kind":"set","field":"value.big","source":{"from":"constant","value":true}}],"elseSteps":[{"kind":"remove","fields":["value.n"]}]}]`, in: `{"n":1}`, want: `[{}]`},
		{name: "if_else no else", steps: `[{"kind":"if_else","condition":{"type":"cond","field":"value.n","op":"exists"},"thenSteps":[{"kind":"drop_if","condition":{"type":"cond","field":"value.n","op":"eq","value":1}}]}]`, in: `{"m":1}`, want: `[{"m":1}]`},
		{name: "if_else nested drop", steps: `[{"kind":"if_else","condition":{"type":"cond","field":"value.n","op":"exists"},"thenSteps":[{"kind":"drop_if","condition":{"type":"cond","field":"value.n","op":"eq","value":1}}]}]`, in: `{"n":1}`, want: `[]`},
		{name: "split_array", steps: `[{"kind":"split_array","field":"value.items","to":"value.item"},{"kind":"remove","fields":["value.items"]}]`, in: `{"id":7,"items":[{"p":1},{"p":2}]}`, want: `[{"id":7,"item":{"p":1}},{"id":7,"item":{"p":2}}]`},
		{name: "split_array in place", steps: `[{"kind":"split_array","field":"value.items"}]`, in: `{"items":[1,2,3]}`, want: `[{"items":1},{"items":2},{"items":3}]`},
		{name: "split_array empty drops", steps: `[{"kind":"split_array","field":"value.items"}]`, in: `{"items":[]}`, want: `[]`},
		{name: "split_array not array", steps: `[{"kind":"split_array","field":"value.items"}]`, in: `{"items":1}`, wantErr: step.ReasonTypeMismatch},
		{name: "wrap", steps: `[{"kind":"wrap","field":"value.payload.data"}]`, in: `{"a":1}`, want: `[{"payload":{"data":{"a":1}}}]`},
		{name: "unwrap", steps: `[{"kind":"unwrap","field":"value.payload"}]`, in: `{"payload":{"a":1},"meta":2}`, want: `[{"a":1}]`},
		{name: "unwrap missing strict", steps: `[{"kind":"unwrap","field":"value.payload","strict":true}]`, in: `{}`, wantErr: step.ReasonFieldMissing},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { runMulti(t, c) })
	}
}

func TestRecipesBatchF(t *testing.T) {
	cases := []multiCase{
		{name: "header_add constant", steps: `[{"kind":"header_add","header":"x-source","source":{"from":"constant","value":"router"}}]`, in: `{}`, want: `[{}]`,
			wantHeaders: map[string]string{"h": "hv", "trace.id": "t-1", "x-source": "router"}},
		{name: "header_add replaces", steps: `[{"kind":"header_add","header":"h","source":{"from":"formula","expr":"concat(\"v\", value.n)"}}]`, in: `{"n":2}`, want: `[{"n":2}]`,
			wantHeaders: map[string]string{"h": "v2", "trace.id": "t-1"}},
		{name: "header_remove dotted name", steps: `[{"kind":"header_remove","header":"trace.id"}]`, in: `{}`, want: `[{}]`, wantHeaders: map[string]string{"h": "hv"}},
		{name: "header_remove missing strict", steps: `[{"kind":"header_remove","header":"nope","strict":true}]`, in: `{}`, wantErr: step.ReasonFieldMissing},
		{name: "header_rename", steps: `[{"kind":"header_rename","header":"h","newHeader":"h-new"}]`, in: `{}`, want: `[{}]`, wantHeaders: map[string]string{"h-new": "hv", "trace.id": "t-1"}},
		{name: "header_copy_to_payload", steps: `[{"kind":"header_copy_to_payload","header":"trace.id","field":"value.trace"}]`, in: `{}`, want: `[{"trace":"t-1"}]`,
			wantHeaders: map[string]string{"h": "hv", "trace.id": "t-1"}},
		{name: "header_copy_from_payload", steps: `[{"kind":"header_copy_from_payload","field":"value.id","header":"id"}]`, in: `{"id":42}`, want: `[{"id":42}]`,
			wantHeaders: map[string]string{"h": "hv", "trace.id": "t-1", "id": "42"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { runMulti(t, c) })
	}
}

func TestMapper(t *testing.T) {
	cases := []multiCase{
		{name: "empty start nests and renames", steps: `[{"kind":"map","start":"empty","target":{"fields":{
			"customer":{"fields":{"id":{"source":{"from":"field","path":"value.cust_id"}},"name":{"source":{"from":"formula","expr":"concat(value.first, \" \", value.last)"}}}},
			"source":{"source":{"from":"constant","value":"crm"}},
			"topic":{"source":{"from":"metadata","name":"topic"}}}}}]`,
			in: `{"cust_id":"C1","first":"Ada","last":"L","x":1}`, want: `[{"customer":{"id":"C1","name":"Ada L"},"source":"crm","topic":"in"}]`},
		{name: "copy_input removes and moves", steps: `[{"kind":"map","target":{"fields":{
			"cust_id":{"remove":true},"customerId":{"source":{"from":"field","path":"value.cust_id"}},"meta":{"fields":{"old":{"remove":true}}}}}}]`,
			in: `{"cust_id":"C1","keep":true,"meta":{"old":1,"new":2}}`, want: `[{"customerId":"C1","keep":true,"meta":{"new":2}}]`},
		{name: "missing leaf skipped", steps: `[{"kind":"map","start":"empty","target":{"fields":{"a":{"source":{"from":"field","path":"value.nope"}},"b":{"source":{"from":"constant","value":1}}}}}]`,
			in: `{}`, want: `[{"b":1}]`},
		{name: "missing leaf strict", steps: `[{"kind":"map","strict":true,"start":"empty","target":{"fields":{"a":{"source":{"from":"field","path":"value.nope"}}}}}]`,
			in: `{}`, wantErr: step.ReasonFieldMissing},
		{name: "formula failure", steps: `[{"kind":"map","target":{"fields":{"a":{"source":{"from":"formula","expr":"1/value.z"}}}}}]`,
			in: `{"z":0}`, wantErr: step.ReasonFormulaFailed},
		{name: "root source", steps: `[{"kind":"map","start":"empty","target":{"source":{"from":"field","path":"value.inner"}}}]`,
			in: `{"inner":{"a":1}}`, want: `[{"a":1}]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { runMulti(t, c) })
	}
}

func TestP2CompileIssues(t *testing.T) {
	cases := map[string]config.Pointer{
		`[{"kind":"redact_pattern","field":"value.a","pattern":"(["}]`:                      "/branches/0/transform/steps/0/pattern",
		`[{"kind":"format_date","field":"value.a","layout":"date","timezone":"Mars/Base"}]`: "/branches/0/transform/steps/0/timezone",
		`[{"kind":"wrap","field":"value"}]`:                                                 "/branches/0/transform/steps/0/field",
		`[{"kind":"unwrap","field":"headers.x"}]`:                                           "/branches/0/transform/steps/0/field",
		`[{"kind":"lookup","field":"value.a","ref":"countries","version":1}]`:               "/branches/0/transform/steps/0/ref",
		`[{"kind":"if_else","condition":{"type":"cond","field":"value.a","op":"exists"},"thenSteps":[{"kind":"rename","from":"value..a","to":"value.b"}]}]`: "/branches/0/transform/steps/0/thenSteps/0/from",
		`[{"kind":"map","target":{"fields":{"a":{"fields":{"b":{"source":{"from":"formula","expr":"nope("}}}}}}}]`:                                          "/branches/0/transform/steps/0/target/fields/a/fields/b/source/expr",
		`[{"kind":"header_add","header":"x","source":{"from":"formula","expr":"upper()"}}]`:                                                                 "/branches/0/transform/steps/0/source/expr",
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

func TestNestedStepErrorPath(t *testing.T) {
	p := enginetest.MustCompile(t, enginetest.RouteJSON("null", `[{"id":"cond","kind":"if_else","condition":{"type":"cond","field":"value.a","op":"exists"},
		"thenSteps":[{"id":"r","kind":"rename","from":"value.z","to":"value.y","strict":true}]}]`))
	ev := enginetest.MustDecode(t, p, `{"a":1}`)
	_, err := p.Process(context.Background(), ev, nil)
	var pe *engine.ProcessError
	if !errors.As(err, &pe) || pe.StepID != "cond" {
		t.Fatalf("want error of outer step, got %v", err)
	}
	var se *step.StepError
	if !errors.As(err, &se) || se.Reason != step.ReasonFieldMissing {
		t.Fatalf("want nested field_missing cause, got %v", err)
	}
}
