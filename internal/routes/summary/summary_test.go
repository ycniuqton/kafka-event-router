package summary

import (
	"encoding/json"
	"testing"

	"github.com/eventrouter/router/pkg/config"
)

func node(t *testing.T, raw string) *config.FilterNode {
	t.Helper()
	var n config.FilterNode
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatal(err)
	}
	return &n
}

func TestCondTextEveryOperator(t *testing.T) {
	cases := map[string]string{
		`{"type":"cond","field":"value.eventType","op":"eq","value":"OrderPaid"}`:          "eventType is OrderPaid",
		`{"type":"cond","field":"value.eventType","op":"ne","value":"X"}`:                  "eventType is not X",
		`{"type":"cond","field":"value.amount","op":"gt","value":100}`:                     "amount > 100",
		`{"type":"cond","field":"value.amount","op":"gte","value":1}`:                      "amount ≥ 1",
		`{"type":"cond","field":"value.amount","op":"lt","value":2}`:                       "amount < 2",
		`{"type":"cond","field":"value.amount","op":"lte","value":3}`:                      "amount ≤ 3",
		`{"type":"cond","field":"value.s","op":"contains","value":"x"}`:                    "s contains x",
		`{"type":"cond","field":"value.s","op":"starts_with","value":"x"}`:                 "s starts with x",
		`{"type":"cond","field":"value.s","op":"ends_with","value":"x"}`:                   "s ends with x",
		`{"type":"cond","field":"value.c","op":"in","values":["DE","FR"]}`:                 "c is one of DE, FR",
		`{"type":"cond","field":"value.c","op":"not_in","values":["DE"]}`:                  "c is not one of DE",
		`{"type":"cond","field":"value.c","op":"exists"}`:                                  "c exists",
		`{"type":"cond","field":"value.c","op":"not_exists"}`:                              "c does not exist",
		`{"type":"cond","field":"value.c","op":"is_empty"}`:                                "c is empty",
		`{"type":"cond","field":"value.c","op":"regex","value":"^a"}`:                      "c matches ^a",
		`{"type":"cond","field":"value.n","op":"between","from":1,"to":5}`:                 "n is between 1 and 5",
		`{"type":"cond","field":"value.t","op":"before","value":"2026-01-01"}`:             "t is before 2026-01-01",
		`{"type":"cond","field":"value.t","op":"after","value":"2026-01-01"}`:              "t is after 2026-01-01",
		`{"type":"cond","field":"value.t","op":"within_last","amount":5,"unit":"minutes"}`: "t is within the last 5 minutes",
		`{"type":"cond","field":"key","op":"eq","value":null}`:                             "key is null",
	}
	for raw, want := range cases {
		if got := (FilterSummarizer{}).Text(node(t, raw)); got != want {
			t.Errorf("%s: got %q want %q", raw, got, want)
		}
	}
}

func TestHeadline(t *testing.T) {
	and := node(t, `{"type":"and","children":[
		{"type":"cond","field":"value.eventType","op":"eq","value":"OrderPaid"},
		{"type":"cond","field":"value.amount","op":"gt","value":100},
		{"type":"or","children":[{"type":"cond","field":"value.a","op":"exists"},{"type":"expr","code":"event.value.b > 1"}]}]}`)
	h := FilterSummarizer{}.Headline(and)
	if h.Headline != "eventType is OrderPaid" || h.More != 2 {
		t.Fatalf("%+v", h)
	}
	if got := (FilterSummarizer{}).Text(and); got != "eventType is OrderPaid and amount > 100 and (a exists or expression: event.value.b > 1)" {
		t.Fatal(got)
	}
	not := node(t, `{"type":"not","child":{"type":"cond","field":"value.a","op":"exists"}}`)
	if h := (FilterSummarizer{}).Headline(not); h.Headline != "not a exists" || h.More != 0 {
		t.Fatalf("%+v", h)
	}
	if (FilterSummarizer{}).Headline(nil) != nil {
		t.Fatal("nil filter has no headline")
	}
}

func TestTransformClassifier(t *testing.T) {
	cfg := func(raw string) *config.RouteConfig {
		var c config.RouteConfig
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatal(err)
		}
		return &c
	}
	cases := map[string]TransformIndicator{
		`{"branches":[{"id":"main"}]}`:                                                         TransformNone,
		`{"branches":[{"id":"main","transform":{"steps":[{"kind":"rename"}]}}]}`:               TransformVisual,
		`{"branches":[{"id":"main","transform":{"steps":[{"kind":"rename"},{"kind":"js"}]}}]}`: TransformJS,
		`{"branches":[{"id":"a"},{"id":"b","transform":{"steps":[{"kind":"library"}]}}]}`:      TransformVisual,
	}
	for raw, want := range cases {
		if got := (TransformClassifier{}).Classify(cfg(raw)); got != want {
			t.Errorf("%s: got %s want %s", raw, got, want)
		}
	}
	if (TransformClassifier{}).Classify(nil) != TransformNone {
		t.Fatal("nil config")
	}
}
