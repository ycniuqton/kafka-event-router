package platform

import (
	"fmt"

	"github.com/eventrouter/router/test/stress/kit/scenario"
)

// RouteDoc is the router/v1 vocabulary of drafts built by the runner.
var RouteDoc = struct {
	APIVersion, MainBranch, JSON string
}{APIVersion: "router/v1", MainBranch: "main", JSON: "json"}

// RouteTopics are the Kafka names one route uses.
type RouteTopics struct {
	Name   string
	Input  string
	Output string
	DLQ    string
	Group  string
}

// AlwaysTrue are filter conditions that hold for every event of the default payload template, cycled to build an
// N-condition AND that filters nothing (rule complexity is measured, not selectivity).
var AlwaysTrue = []map[string]any{
	{"type": "cond", "field": "value.type", "op": "eq", "value": "order"},
	{"type": "cond", "field": "value.amount", "op": "gt", "value": 0},
	{"type": "cond", "field": "value.currency", "op": "eq", "value": "USD"},
	{"type": "cond", "field": "value.seq", "op": "gte", "value": 0},
	{"type": "cond", "field": "value.customer.country", "op": "in", "values": []string{"VN", "US", "DE"}},
	{"type": "cond", "field": "value.eventType", "op": "starts_with", "value": "Order"},
	{"type": "cond", "field": "value.stream", "op": "exists"},
	{"type": "cond", "field": "value.customer.email", "op": "contains", "value": "@"},
	{"type": "cond", "field": "value.orderId", "op": "ne", "value": ""},
	{"type": "cond", "field": "value.amount", "op": "lt", "value": 1e9},
}

// JSCode are the JavaScript step bodies per mode.
var JSCode = map[scenario.JSMode]string{
	scenario.JSIdentity:   "function transform(event, ctx) { return event; }",
	scenario.JSThrowOnBad: `function transform(event, ctx) { if (event.value.bad) { throw new Error("bad event " + event.value.seq); } return event; }`,
}

// StepGenerators build the i-th generated recipe step of a kind.
var StepGenerators = map[string]func(i int) map[string]any{
	"set": func(i int) map[string]any {
		return map[string]any{"kind": "set", "field": fmt.Sprintf("value.stress_f%d", i), "source": map[string]any{"from": "constant", "value": i}}
	},
	"mask": func(int) map[string]any {
		return map[string]any{"kind": "mask", "field": "value.customer.email", "style": "email"}
	},
}

// DraftBuilder turns a scenario route spec into a router/v1 draft.
type DraftBuilder struct {
	Spec         scenario.RouteSpec
	ConnectionID string
}

// Build returns the draft of one route.
func (b DraftBuilder) Build(t RouteTopics) (map[string]any, error) {
	steps, err := b.steps()
	if err != nil {
		return nil, err
	}
	branch := map[string]any{
		"id": RouteDoc.MainBranch, "transform": map[string]any{"steps": steps},
		"destination": map[string]any{
			"connectionId": b.ConnectionID, "topic": t.Output, "key": map[string]string{"mode": "keep"}, "partitioning": "by_key",
			"headers": map[string]string{"mode": "keep_all"}, "format": map[string]string{"type": RouteDoc.JSON},
		},
	}
	if f := b.filter(); f != nil {
		branch["filter"] = f
	}
	doc := map[string]any{
		"apiVersion": RouteDoc.APIVersion, "name": t.Name,
		"source": map[string]any{
			"connectionId": b.ConnectionID, "topics": []string{t.Input}, "format": map[string]string{"type": RouteDoc.JSON},
			"startFrom": map[string]string{"mode": b.Spec.StartFrom}, "consumerGroup": t.Group,
		},
		"errorPolicy": map[string]any{"onError": b.Spec.OnError, "dlqTopic": t.DLQ},
		"delivery":    map[string]string{"mode": b.Spec.Delivery},
		"branches":    []any{branch},
	}
	if b.Spec.MaxInFlight > 0 {
		doc["limits"] = map[string]int{"maxInFlight": b.Spec.MaxInFlight}
	}
	for k, v := range b.Spec.Extra {
		doc[k] = v
	}
	return doc, nil
}

func (b DraftBuilder) filter() map[string]any {
	if b.Spec.Filter.Raw != nil {
		return b.Spec.Filter.Raw
	}
	n := b.Spec.Filter.Conditions
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return AlwaysTrue[0]
	}
	children := make([]any, n)
	for i := range children {
		children[i] = AlwaysTrue[i%len(AlwaysTrue)]
	}
	return map[string]any{"type": "and", "children": children}
}

func (b DraftBuilder) steps() ([]any, error) {
	out := []any{}
	for _, s := range b.Spec.Steps.Raw {
		out = append(out, s)
	}
	gen, ok := StepGenerators[b.Spec.Steps.Kind]
	if !ok && b.Spec.Steps.Count > 0 {
		return nil, fmt.Errorf("unknown generated step kind %q", b.Spec.Steps.Kind)
	}
	for i := 0; i < b.Spec.Steps.Count; i++ {
		out = append(out, gen(i))
	}
	if code, ok := JSCode[b.Spec.JS]; ok {
		out = append(out, map[string]any{"id": "stress-js", "kind": "js", "code": code})
	}
	return out, nil
}
