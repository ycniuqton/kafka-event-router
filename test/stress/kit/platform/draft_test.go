package platform

import (
	"testing"

	"github.com/eventrouter/router/test/stress/kit/scenario"
)

var topics = RouteTopics{Name: "stress-x-r0", Input: "stress-x-in", Output: "stress-x-out", DLQ: "stress-x-dlq", Group: "stress-x-g"}

func TestDraftCarriesStressNamesAndLimits(t *testing.T) {
	spec := scenario.BuiltIn.Case.Routes
	spec.MaxInFlight, spec.Filter.Conditions, spec.Steps.Count, spec.JS = 250, 12, 3, scenario.JSThrowOnBad
	d, err := DraftBuilder{Spec: spec, ConnectionID: "c1"}.Build(topics)
	if err != nil {
		t.Fatal(err)
	}
	src := d["source"].(map[string]any)
	if src["consumerGroup"] != "stress-x-g" || d["errorPolicy"].(map[string]any)["dlqTopic"] != "stress-x-dlq" {
		t.Errorf("names = %v / %v", src, d["errorPolicy"])
	}
	if d["limits"].(map[string]int)["maxInFlight"] != 250 {
		t.Errorf("limits = %v", d["limits"])
	}
	branch := d["branches"].([]any)[0].(map[string]any)
	if n := len(branch["filter"].(map[string]any)["children"].([]any)); n != 12 {
		t.Errorf("conditions = %d", n)
	}
	steps := branch["transform"].(map[string]any)["steps"].([]any)
	if len(steps) != 4 || steps[3].(map[string]any)["kind"] != "js" {
		t.Errorf("steps = %v", steps)
	}
}

func TestDraftWithoutComplexity(t *testing.T) {
	d, err := DraftBuilder{Spec: scenario.BuiltIn.Case.Routes, ConnectionID: "c1"}.Build(topics)
	if err != nil {
		t.Fatal(err)
	}
	branch := d["branches"].([]any)[0].(map[string]any)
	if _, ok := branch["filter"]; ok {
		t.Error("filter set without conditions")
	}
	if _, ok := d["limits"]; ok {
		t.Error("limits set without maxInFlight")
	}
	spec := scenario.BuiltIn.Case.Routes
	spec.Steps = scenario.StepsSpec{Count: 1, Kind: "nope"}
	if _, err := (DraftBuilder{Spec: spec}).Build(topics); err == nil {
		t.Error("unknown step kind accepted")
	}
}
