package infer_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/assist/infer"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/trace"
)

func inferrer(t testing.TB, order ...infer.Reason) *infer.MappingInferrer {
	t.Helper()
	if len(order) == 0 {
		order = infer.DefaultOrder
	}
	m, err := infer.DefaultMatcherManager(order)
	if err != nil {
		t.Fatal(err)
	}
	return infer.NewMappingInferrer(m, infer.DefaultLimits)
}

// apply runs the proposed steps through pkg/engine and returns the output value.
func apply(t *testing.T, value string, steps []json.RawMessage) any {
	t.Helper()
	raw, _ := json.Marshal(steps)
	doc := `{"apiVersion":"router/v1","name":"infer","source":{"connectionId":"c","topics":["in"],"format":{"type":"json"}},
	  "branches":[{"id":"main","transform":{"steps":` + string(raw) + `},"destination":{"connectionId":"c","topic":"out"}}]}`
	parsed, err := config.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	p, err := compile.NewDefaultCompiler().Compile(context.Background(), parsed, engine.CompileOptions{})
	if err != nil {
		t.Fatalf("compile %s: %v", raw, err)
	}
	var v any
	_ = json.Unmarshal([]byte(value), &v)
	outs, err := p.Process(context.Background(), engine.Event{Value: v}, trace.NoopTracer{})
	if err != nil || len(outs) != 1 {
		t.Fatalf("process: %v %v", outs, err)
	}
	return outs[0].Value
}

func TestMappingInferrerProducesTheExample(t *testing.T) {
	cases := []struct {
		name, input, desired string
		reasons              map[string]infer.Reason
		unmapped             []string
	}{
		{"rename same parent", `{"cust_id":"c-1","amount":12}`, `{"customerId":"c-1","amount":12}`,
			map[string]infer.Reason{"value.customerId": infer.ReasonValue, "value.amount": infer.ReasonValue}, []string{}},
		{"nesting move and removal", `{"customer":{"id":"c-1","email":"a@x"},"debug":true}`, `{"customerId":"c-1","contact":{"email":"a@x"}}`,
			map[string]infer.Reason{"value.customerId": infer.ReasonValue, "value.contact.email": infer.ReasonValue}, []string{"value.debug"}},
		{"constant and name", `{"Order_Id":"o-1","total":"12.5"}`, `{"orderId":"o-1","total":12.5,"source":"web"}`,
			map[string]infer.Reason{"value.orderId": infer.ReasonValue, "value.total": infer.ReasonConstant, "value.source": infer.ReasonConstant}, []string{}},
		{"one source two targets", `{"id":7}`, `{"id":7,"ref":{"id":7}}`,
			map[string]infer.Reason{"value.id": infer.ReasonValue, "value.ref.id": infer.ReasonValue}, []string{}},
		{"numbers compare by value", `{"a":1.0,"b":"x"}`, `{"c":1}`, map[string]infer.Reason{"value.c": infer.ReasonValue}, []string{"value.b"}},
	}
	for _, c := range cases {
		res, err := inferrer(t).Infer(infer.Request{Input: json.RawMessage(`{"key":"k","value":` + c.input + `}`), DesiredOutput: json.RawMessage(c.desired)})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for _, m := range res.Mappings {
			if want, ok := c.reasons[m.Target]; !ok || want != m.Reason {
				t.Errorf("%s: mapping %+v, want reason %q", c.name, m, want)
			}
		}
		if !reflect.DeepEqual(res.Unmapped, c.unmapped) {
			t.Errorf("%s: unmapped %v, want %v", c.name, res.Unmapped, c.unmapped)
		}
		var want any
		_ = json.Unmarshal([]byte(c.desired), &want)
		if got := apply(t, c.input, res.Steps); !reflect.DeepEqual(normalise(got), normalise(want)) {
			t.Errorf("%s: engine output %v, want %v (steps %s)", c.name, got, want, res.Steps)
		}
	}
}

func normalise(v any) string {
	raw, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(raw, &out)
	again, _ := json.Marshal(out)
	return string(again)
}

func TestMatchersAmbiguityAndOrder(t *testing.T) {
	in := json.RawMessage(`{"value":{"a":"x","b":"x","flag":true,"status":"new"}}`)
	res, err := inferrer(t).Infer(infer.Request{Input: in, DesiredOutput: json.RawMessage(`{"b":"x","state":"open"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Mappings[0]; m.Source != "value.b" || m.Confidence != infer.ConfidenceAmbiguous {
		t.Errorf("ambiguous equal value: %+v", m)
	}
	if m := res.Mappings[1]; m.Reason != infer.ReasonType || m.Confidence != infer.ConfidenceType {
		t.Errorf("type shape fallback: %+v", m)
	}
	res, _ = inferrer(t, infer.ReasonName).Infer(infer.Request{Input: in, DesiredOutput: json.RawMessage(`{"STATUS":"x"}`)})
	if m := res.Mappings[0]; m.Reason != infer.ReasonName || m.Source != "value.status" {
		t.Errorf("name only order: %+v", m)
	}
	if _, err := infer.DefaultMatcherManager([]infer.Reason{"guess"}); err == nil {
		t.Error("unknown matcher accepted")
	}
}

func TestInferRejectsBadRequests(t *testing.T) {
	cases := []struct {
		input, desired string
		want           error
	}{
		{`{"value":{"a":1}}`, `[1,2]`, infer.ErrDesiredNotObject},
		{`{"value":{"a":1}}`, `"text"`, infer.ErrDesiredNotObject},
		{`{"value":[1]}`, `{"a":1}`, infer.ErrInputNotEvent},
		{`7`, `{"a":1}`, infer.ErrInputNotEvent},
	}
	for _, c := range cases {
		if _, err := inferrer(t).Infer(infer.Request{Input: json.RawMessage(c.input), DesiredOutput: json.RawMessage(c.desired)}); !errors.Is(err, c.want) {
			t.Errorf("%s → %s: err %v, want %v", c.input, c.desired, err, c.want)
		}
	}
}

func TestNamesAndPaths(t *testing.T) {
	if (infer.Names{}).Key("Customer_ID") != (infer.Names{}).Key("customer-id") {
		t.Error("names must normalise case, '_' and '-'")
	}
	if (infer.PathRules{}).Addressable([]string{"a.b"}) || !(infer.PathRules{}).Addressable([]string{"a", "b_c"}) {
		t.Error("addressable paths")
	}
	res, _ := inferrer(t).Infer(infer.Request{Input: json.RawMessage(`{"value":{"a.b":1}}`), DesiredOutput: json.RawMessage(`{"c":1}`)})
	if strings.Contains(string(res.Steps[0]), "a.b") {
		t.Errorf("unaddressable key used: %s", res.Steps)
	}
}

// FuzzInferMapping: random JSON trees never panic and every proposed step is valid JSON.
func FuzzInferMapping(f *testing.F) {
	f.Add([]byte(`{"a":{"b":[1,2]},"c":null}`), []byte(`{"x":{"y":1},"c":null}`))
	f.Add([]byte(`{"":1,"a.b":{"[":2}}`), []byte(`{"z":{}}`))
	f.Add([]byte(`{}`), []byte(`{}`))
	m := inferrer(f)
	f.Fuzz(func(t *testing.T, input, desired []byte) {
		res, err := m.Infer(infer.Request{Input: append(append([]byte(`{"value":`), input...), '}'), DesiredOutput: desired})
		if err != nil {
			return
		}
		for _, s := range res.Steps {
			if !json.Valid(s) {
				t.Fatalf("invalid step %s", s)
			}
		}
	})
}
