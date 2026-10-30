package dlq

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

var jsonSource = SourceFormat{Key: config.Format{Type: config.FormatTypeString}, Value: config.Format{Type: config.FormatTypeJSON}}

func editOriginal() record.Record {
	r := failure(1, record.ErrorStep)
	r.Key = []byte("ord_123")
	r.Value = []byte(`{"orderId":"ord_123","items":null,"amount":19.5,"customer":{"email":"a@b.io","vip":true}}`)
	r.Headers = []engine.Header{{Key: "traceId", Value: []byte("ab12")}, {Key: "tenant", Value: []byte("acme")}}
	return r
}

func jsonValue(t *testing.T, raw string) *EncodedValue {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	return &EncodedValue{Encoding: EncodingJSON, Data: v}
}

func TestEditPlannerReplacesAndDiffs(t *testing.T) {
	orig := editOriginal()
	edit := &RecordEdit{
		Value:   jsonValue(t, `{"orderId":"ord_123","items":[{"sku":"a"}],"amount":19.5,"customer":{"email":"a@b.io"}}`),
		Headers: &[]EncodedHeader{{Key: "traceId", Value: "zz99"}, {Key: "router.version", Value: "9"}, {Key: "new", Value: "1"}},
	}
	plan, err := EditPlanner{Masker: TypePreservingMasker{}}.Plan(EditInput{Original: orig, Edit: edit, Format: jsonSource})
	if err != nil || !plan.Edited {
		t.Fatalf("plan %v %+v", err, plan)
	}
	want := []DiffEntry{{"/value/customer/vip", DiffRemove}, {"/value/items", DiffAdd}, {"/headers/new", DiffAdd},
		{"/headers/tenant", DiffRemove}, {"/headers/traceId", DiffReplace}}
	if !reflect.DeepEqual(plan.Diff, want) {
		t.Fatalf("diff %+v", plan.Diff)
	}
	if string(plan.Record.Key) != "ord_123" || len(plan.Record.Headers) != 2 || plan.Record.Ref != orig.Ref {
		t.Fatalf("record %+v", plan.Record)
	}
}

func TestEditPlannerRestoresUntouchedMaskedValues(t *testing.T) {
	orig := editOriginal()
	shown := PayloadMasker{Masker: TypePreservingMasker{}}.Bytes(orig.Value)
	var tree map[string]any
	_ = json.Unmarshal(shown, &tree)
	tree["items"] = []any{"x"}
	edit := &RecordEdit{Value: &EncodedValue{Encoding: EncodingJSON, Data: tree},
		Headers: &[]EncodedHeader{{Key: "traceId", Value: TypePreservingMasker{}.Text("ab12")}, {Key: "tenant", Value: "other"}}}
	plan, err := EditPlanner{Masker: TypePreservingMasker{}}.Plan(EditInput{Original: orig, Edit: edit, Masked: true, Format: jsonSource})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(plan.Record.Value, &got)
	customer := got["customer"].(map[string]any)
	if got["orderId"] != "ord_123" || got["amount"] != 19.5 || customer["email"] != "a@b.io" || customer["vip"] != true {
		t.Fatalf("masked leaves not restored: %s", plan.Record.Value)
	}
	hs := HeaderTree{}.Of(plan.Record.Headers)
	if hs["traceId"] != "ab12" || hs["tenant"] != "other" {
		t.Fatalf("headers %+v", plan.Record.Headers)
	}
	if !reflect.DeepEqual(plan.Diff, []DiffEntry{{"/value/items", DiffAdd}, {"/headers/tenant", DiffReplace}}) || !plan.Masked {
		t.Fatalf("diff %+v", plan.Diff)
	}
}

func TestEditPlannerValidation(t *testing.T) {
	orig := editOriginal()
	p := EditPlanner{Masker: TypePreservingMasker{}}
	cases := []struct {
		edit   *RecordEdit
		format SourceFormat
		want   error
	}{
		{&RecordEdit{Value: &EncodedValue{Encoding: EncodingUTF8, Data: "{not json"}}, jsonSource, ErrInvalidEdit},
		{&RecordEdit{Value: &EncodedValue{Encoding: EncodingBase64, Data: "%%%"}}, jsonSource, ErrInvalidEdit},
		{&RecordEdit{Value: &EncodedValue{Encoding: EncodingUTF8, Data: 5}}, jsonSource, ErrInvalidEdit},
		{&RecordEdit{Value: &EncodedValue{Encoding: "yaml", Data: "a"}}, jsonSource, ErrInvalidEdit},
		{&RecordEdit{Value: &EncodedValue{Encoding: EncodingAvro, Data: map[string]any{}}}, jsonSource, ErrEditUnsupported},
		{&RecordEdit{Value: jsonValue(t, `{"a":1}`)}, SourceFormat{Value: config.Format{Type: config.FormatTypeAvro}}, ErrEditUnsupported},
		{&RecordEdit{Value: &EncodedValue{Encoding: EncodingUTF8, Data: string(make([]byte, MaxEditBytes+1))}},
			SourceFormat{Value: config.Format{Type: config.FormatTypeString}}, ErrEditTooBig},
	}
	for i, c := range cases {
		_, err := p.Plan(EditInput{Original: orig, Edit: c.edit, Format: c.format})
		if !errors.Is(err, c.want) {
			t.Errorf("case %d: got %v want %v", i, err, c.want)
		}
	}
	plan, err := p.Plan(EditInput{Original: orig, Edit: &RecordEdit{}, Format: jsonSource})
	if err != nil || plan.Edited || len(plan.Diff) != 0 {
		t.Fatalf("empty edit %v %+v", err, plan)
	}
	plan, err = p.Plan(EditInput{Original: orig, Edit: &RecordEdit{Key: &EncodedValue{Encoding: EncodingJSON}}, Format: jsonSource})
	if err != nil || plan.Record.Key != nil || plan.Diff[0] != (DiffEntry{"/key", DiffRemove}) {
		t.Fatalf("null key %v %+v", err, plan)
	}
}

func TestStructuralDiffArraysAndPointers(t *testing.T) {
	before := map[string]any{"a/b": []any{1.0, 2.0, 3.0}, "x": "y"}
	after := map[string]any{"a/b": []any{1.0, 5.0}, "x": "y", "m~n": true}
	got := StructuralDiff{}.Of("/value", before, after)
	want := []DiffEntry{{"/value/a~1b/1", DiffReplace}, {"/value/a~1b/2", DiffRemove}, {"/value/m~0n", DiffAdd}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diff %+v", got)
	}
}
