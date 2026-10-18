package sandbox

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eventrouter/router/pkg/sandbox/host"
)

func TestRuntimeInfoShape(t *testing.T) {
	info := InfoBuilder{}.Build(EngineQuickJSWazero, NewLimitPolicy(LimitMaximum))
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	for _, key := range []string{"engine", "determinismVersion", "limits", "helpers", "typings", "safetyRules"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("runtime-info misses %q", key)
		}
	}
	want := LimitsDoc{JSTimeoutMs: 50, JSMemoryMb: 64, JSMaxOutputKb: 1024, LogKbPerCall: 16}
	if info.Limits.Defaults != want {
		t.Errorf("defaults = %+v, want %+v", info.Limits.Defaults, want)
	}
	if info.Limits.Maxima.JSTimeoutMs != 1000 || info.Limits.Maxima.JSMemoryMb != 256 || info.Limits.Maxima.JSMaxOutputKb != 8192 {
		t.Errorf("maxima = %+v", info.Limits.Maxima)
	}
	if len(info.Helpers) != 8 || len(info.SafetyRules) != 5 {
		t.Errorf("helpers=%d rules=%d", len(info.Helpers), len(info.SafetyRules))
	}
}

func TestTypingsCoverEveryHelper(t *testing.T) {
	docs := host.NewStandardRegistry(nil).Docs()
	ts := TypingsBuilder{}.Build(docs)
	for _, frag := range []string{"interface RouterEvent", "interface RouterCtx", "mask: {", "email: (value: unknown) => string;",
		"hash: (value: unknown) => string;", "now: () => string;", "lookup: (table: string, key: unknown) => unknown;",
		"declare const ctx: RouterCtx;", "declare function transform(",
		"type RouterLong = { $long: string };", "type RouterDecimal = { $decimal: string };", "type RouterTimestamp = { $timestamp: string };",
		"type RouterTagged = RouterLong | RouterBytes | RouterDecimal | RouterTimestamp;"} {
		if !strings.Contains(ts, frag) {
			t.Errorf("typings miss %q:\n%s", frag, ts)
		}
	}
}
