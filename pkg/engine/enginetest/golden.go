// Package enginetest provides deterministic fixtures shared by engine golden tests,
// routerctl tests and benchmarks: fixed clock and the fake scripts used by goldens.
package enginetest

import (
	"context"
	"fmt"
	"time"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/engine/scripttest"
)

// GoldenNow is the frozen processing time of every golden case.
var GoldenNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// GoldenScript names fake scripts registered for golden routes.
type GoldenScript string

const (
	ScriptSplitItems GoldenScript = "golden:split-items"
	ScriptHighValue  GoldenScript = "golden:high-value"
	ScriptCountry    GoldenScript = "golden:lookup-country"
)

// GoldenOptions returns compile options with a fixed clock and the golden fake scripts.
func GoldenOptions() engine.CompileOptions {
	rt := scripttest.NewFuncRuntime().
		Register(string(ScriptSplitItems), splitItems{}.Call).
		Register(string(ScriptHighValue), highValue{}.Call).
		RegisterBound(string(ScriptCountry), lookupCountry{}.Bind)
	return engine.CompileOptions{
		Scripts: rt,
		Helpers: ctxhelpers.Set{Clock: ctxhelpers.FixedClock{T: GoldenNow}, Sources: GoldenSources()}.WithDefaults(),
	}
}

// splitItems emits one event per value.items element, keyed by sku.
type splitItems struct{}

func (splitItems) Call(_ context.Context, inv script.Invocation) (script.Result, error) {
	value, _ := inv.Event["value"].(map[string]any)
	items, _ := value["items"].([]any)
	out := make([]any, 0, len(items))
	for _, it := range items {
		item, _ := it.(map[string]any)
		out = append(out, map[string]any{"key": item["sku"], "value": map[string]any{"orderId": value["orderId"], "item": item}})
	}
	return script.Result{Value: out, Logs: []string{"split"}}, nil
}

// highValue is the predicate value.amount > 1000.
type highValue struct{}

func (highValue) Call(_ context.Context, inv script.Invocation) (script.Result, error) {
	value, _ := inv.Event["value"].(map[string]any)
	amount, _ := value["amount"].(int64)
	return script.Result{Value: amount > 1000}, nil
}

// Country lookup names: the pinned library table and the fallback of missing codes.
const (
	CountryTable    = "countries"
	CountryField    = "countryName"
	CountryFallback = "Unknown"
)

// lookupCountry mirrors `event.value.countryName = ctx.lookup("countries", event.value.country) ?? "Unknown"`
// with the sandbox's ctx.lookup semantics (key rendered as text, missing → null).
type lookupCountry struct{}

func (lookupCountry) Bind(src script.Source) scripttest.Func {
	return func(_ context.Context, inv script.Invocation) (script.Result, error) {
		value, _ := inv.Event["value"].(map[string]any)
		name, ok := src.Lookups.Lookup(CountryTable, fmt.Sprint(value["country"]))
		if !ok {
			name = CountryFallback
		}
		value[CountryField] = name
		return script.Result{Value: inv.Event}, nil
	}
}
