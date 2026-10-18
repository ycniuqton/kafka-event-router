package preview_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// jsonValue generates decoded JSON values (objects, arrays, strings, numbers, booleans, null).
func jsonValue(depth int) *rapid.Generator[any] {
	leaf := rapid.OneOf(
		rapid.Map(rapid.StringMatching(`[a-zA-Z0-9@._ -]{1,24}`), func(s string) any { return s }),
		rapid.Map(rapid.Float64Range(-1e9, 1e9), func(f float64) any { return f }),
		rapid.Map(rapid.Bool(), func(b bool) any { return b }),
		rapid.Just[any](nil),
	)
	if depth <= 0 {
		return leaf
	}
	obj := rapid.Map(rapid.MapOfN(rapid.StringMatching(`[a-z]{1,6}`), jsonValue(depth-1), 0, 4), func(m map[string]any) any { return m })
	arr := rapid.Map(rapid.SliceOfN(jsonValue(depth-1), 0, 4), func(s []any) any { return s })
	return rapid.OneOf(leaf, obj, arr)
}

// leaves collects scalar leaves of v.
func leaves(v any, out *[]any) {
	switch t := v.(type) {
	case map[string]any:
		for _, c := range t {
			leaves(c, out)
		}
	case []any:
		for _, c := range t {
			leaves(c, out)
		}
	default:
		*out = append(*out, v)
	}
}

// TestMaskersNeverLeakRawValues: no raw string survives as a leaf or substring, numbers become 0 (or a marker).
func TestMaskersNeverLeakRawValues(t *testing.T) {
	maskers := preview.DefaultMaskerManager()
	for _, style := range []preview.MaskStyle{preview.MaskTypePreserving, preview.MaskRedact, preview.MaskPartial} {
		t.Run(string(style), func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				in := jsonValue(3).Draw(rt, "value")
				out := maskers.For(style).Mask(in)
				var raw, masked []any
				leaves(in, &raw)
				leaves(out, &masked)
				encoded, _ := json.Marshal(out)
				for _, r := range raw {
					assertLeafMasked(rt, style, r, masked, string(encoded))
				}
			})
		})
	}
}

func assertLeafMasked(rt *rapid.T, style preview.MaskStyle, raw any, masked []any, encoded string) {
	switch r := raw.(type) {
	case string:
		tokens := string(preview.RedactedValue) + string(preview.MaskedString)
		if len([]rune(r)) >= 6 && style != preview.MaskPartial && !strings.Contains(tokens, r) && strings.Contains(encoded, r) {
			rt.Fatalf("raw string %q survived in %s", r, encoded)
		}
		for _, m := range masked {
			if m == r {
				rt.Fatalf("raw string %q kept as a leaf", r)
			}
		}
	case float64:
		for _, m := range masked {
			if f, ok := m.(float64); ok && f == r && r != 0 {
				rt.Fatalf("raw number %v kept", r)
			}
		}
	}
}

func TestTypePreservingKeepsTypes(t *testing.T) {
	m := preview.DefaultMaskerManager().For("")
	got := m.Mask(map[string]any{"email": "a@b.c", "amount": 12.5, "ok": true, "items": []any{"x", 3.0}, "none": nil, "raw": json.RawMessage(`{"n":5}`)})
	want := `{"amount":0,"email":"••••••","items":["••••••",0],"none":null,"ok":false,"raw":{"n":0}}`
	if b, _ := json.Marshal(got); string(b) != want {
		t.Fatalf("masked = %s", b)
	}
	if p := preview.DefaultMaskerManager().For(preview.MaskPartial).MaskText("alice@example.com"); p != "a•••••••••••••••m" {
		t.Fatalf("partial = %q", p)
	}
	if r := preview.DefaultMaskerManager().For(preview.MaskRedact).Mask(42.0); r != "[redacted]" {
		t.Fatalf("redact = %v", r)
	}
}

func TestResponseMaskerCoversEveryPayloadField(t *testing.T) {
	secret := "card-4111111111111111"
	key := "cust-secret-key"
	rm := preview.NewResponseMasker(preview.DefaultMaskerManager().For(preview.MaskTypePreserving))
	sample := rm.Sample(preview.SampleDTO{
		Key: key, Value: map[string]any{"card": secret, "n": 7.0}, Headers: []engine.HeaderDoc{{Key: "trace", Value: secret}},
		Raw: preview.RawDTO{Key: &key, Value: `{"card":"` + secret + `","n":7}`, Encoding: preview.EncodingUTF8},
	})
	run := rm.Run(preview.RunResultDTO{Results: []preview.EventResultDTO{{Branches: []preview.BranchResultDTO{{
		Steps:   []preview.StepTraceDTO{{Patch: []trace.PatchOp{{Op: "add", Path: "/value/card", Value: secret}}, Logs: []string{"saw " + secret}}},
		Outputs: []preview.OutputDTO{{Key: key, Value: json.RawMessage(`{"card":"` + secret + `"}`), Headers: []engine.HeaderDoc{{Key: "h", Value: secret}}}},
	}}}}})
	schema := rm.Schema(preview.FieldSchema{Root: preview.FieldNode{Children: []preview.FieldNode{{Path: "value.card", TopValues: []preview.ValueCount{{Value: secret, Count: 1}}}}}})
	for name, v := range map[string]any{"sample": sample, "run": run, "schema": schema} {
		b, _ := json.Marshal(v)
		if strings.Contains(string(b), secret) || strings.Contains(string(b), key) {
			t.Fatalf("%s leaks a raw value: %s", name, b)
		}
	}
	if !run.Masked || sample.Raw.Value != `{"card":"••••••","n":0}` {
		t.Fatalf("run masked=%v raw=%s", run.Masked, sample.Raw.Value)
	}
}

func TestMaskPolicyFollowsCallerOrSession(t *testing.T) {
	policy := preview.MaskPolicy{}
	cases := []struct {
		caller preview.Caller
		sess   preview.Session
		want   bool
	}{
		{preview.Caller{}, preview.Session{}, false},
		{preview.Caller{Mask: true}, preview.Session{}, true},
		{preview.Caller{}, preview.Session{Masked: true}, true},
	}
	for i, c := range cases {
		if _, got := policy.For(c.caller, c.sess); got != c.want {
			t.Fatalf("case %d: masked = %v", i, got)
		}
	}
	m, _ := policy.For(preview.Caller{MaskStyle: preview.MaskTypePreserving}, preview.Session{Masked: true, MaskStyle: preview.MaskRedact})
	if got := fmt.Sprint(m.Sample(preview.SampleDTO{Value: 1.0}).Value); got != "[redacted]" {
		t.Fatalf("session style must win: %s", got)
	}
}

// TestResponseMaskerPatchKeepsHeaderNames: step patches mask header values but keep names, like samples and outputs.
func TestResponseMaskerPatchKeepsHeaderNames(t *testing.T) {
	secret := "t-secret-1"
	header := func(name string) map[string]any { return map[string]any{"key": name, "value": secret} }
	doc := map[string]any{"key": secret, "value": map[string]any{"id": secret}, "headers": []any{header("traceId")}, "topic": "orders", "partition": 2.0}
	ops := []trace.PatchOp{
		{Op: "add", Path: "/headers/1", Value: header("source"), HasValue: true},
		{Op: "replace", Path: "/headers/0/key", Value: "trace-id", HasValue: true},
		{Op: "replace", Path: "/headers/0/value", Value: secret, HasValue: true},
		{Op: "replace", Path: "/headers", Value: []any{header("order-id")}, HasValue: true},
		{Op: "add", Path: "/1", Value: doc, HasValue: true},
		{Op: "replace", Path: "", Value: []any{doc}, HasValue: true},
		{Op: "add", Path: "/0/headers/2", Value: header("copied"), HasValue: true},
		{Op: "add", Path: "/value/meta", Value: map[string]any{"key": secret}, HasValue: true},
		{Op: "remove", Path: "/headers/0"},
	}
	rm := preview.NewResponseMasker(preview.DefaultMaskerManager().For(preview.MaskTypePreserving))
	run := rm.Run(preview.RunResultDTO{Results: []preview.EventResultDTO{{Branches: []preview.BranchResultDTO{{Steps: []preview.StepTraceDTO{{Patch: ops}}}}}}})
	got := run.Results[0].Branches[0].Steps[0].Patch
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), secret) {
		t.Fatalf("patch leaks a raw value: %s", b)
	}
	for _, name := range []string{`"source"`, `"trace-id"`, `"order-id"`, `"traceId"`, `"copied"`, `"orders"`} {
		if !strings.Contains(string(b), name) {
			t.Fatalf("patch lost %s: %s", name, b)
		}
	}
	if got[2].Value != string(preview.MaskedString) || got[8].HasValue || got[8].Value != nil {
		t.Fatalf("header value / remove op = %+v / %+v", got[2], got[8])
	}
	if v := got[4].Value.(map[string]any); v["partition"] != 2.0 || v["key"] != string(preview.MaskedString) {
		t.Fatalf("event document = %v", v)
	}
}

func TestEventPointerParts(t *testing.T) {
	cases := []struct {
		path  string
		value any
		want  preview.EventPart
	}{
		{"", map[string]any{}, preview.PartDocument},
		{"", []any{}, preview.PartList},
		{"/2", map[string]any{}, preview.PartDocument},
		{"/headers", nil, preview.PartHeaders},
		{"/0/headers/1", nil, preview.PartHeader},
		{"/headers/1/key", nil, preview.PartHeaderName},
		{"/headers/1/value", nil, preview.PartPayload},
		{"/value/headers", nil, preview.PartPayload},
		{"/key", nil, preview.PartPayload},
		{"/1/timestamp", nil, preview.PartMetadata},
	}
	for _, c := range cases {
		if got := (preview.EventPointer{}).Part(c.path, c.value); got != c.want {
			t.Errorf("%q: part = %v, want %v", c.path, got, c.want)
		}
	}
}
