package enrich_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/enrich"
)

func event() *engine.Event {
	return &engine.Event{
		Value:   map[string]any{"customer": map[string]any{"id": "c 1", "n": int64(7)}, "items": []any{"a", "b"}},
		Headers: []engine.Header{{Key: "tenant", Value: []byte("acme")}},
	}
}

func TestTemplateRender(t *testing.T) {
	cases := []struct {
		raw      string
		text     string
		value    any
		ok       bool
		single   bool
		nFields  int
		segments bool
	}{
		{raw: "/customers/${value.customer.id}", text: "/customers/c%201", value: "/customers/c 1", ok: true, nFields: 1, segments: true},
		{raw: "${value.customer.n}", text: "7", value: int64(7), ok: true, single: true, nFields: 1},
		{raw: "${ value.items }", text: `["a","b"]`, value: []any{"a", "b"}, ok: true, single: true, nFields: 1},
		{raw: "t=${headers.tenant}&n=${value.customer.n}", text: "t=acme&n=7", value: "t=acme&n=7", ok: true, nFields: 2},
		{raw: "plain", text: "plain", value: "plain", ok: true},
		{raw: "x/${value.missing}", ok: false, nFields: 1},
	}
	for _, c := range cases {
		tpl, err := enrich.TemplateParser{}.Parse(c.raw)
		if err != nil {
			t.Fatalf("%q: %v", c.raw, err)
		}
		var esc enrich.Escaper = enrich.PlainText{}
		if c.segments {
			esc = enrich.PathSegment{}
		}
		text, ok := tpl.Text(event(), esc)
		if ok != c.ok || (ok && text != c.text) {
			t.Fatalf("%q text: %q %v", c.raw, text, ok)
		}
		v, okV := tpl.Value(event())
		if okV != c.ok || (okV && !reflect.DeepEqual(v, c.value)) {
			t.Fatalf("%q value: %#v %v", c.raw, v, okV)
		}
		if tpl.Single() != c.single || len(tpl.Fields()) != c.nFields {
			t.Fatalf("%q single=%v fields=%d", c.raw, tpl.Single(), len(tpl.Fields()))
		}
	}
}

func TestTemplateErrors(t *testing.T) {
	for _, raw := range []string{"${value.a", "${nope.a}", "${}"} {
		if _, err := (enrich.TemplateParser{}).Parse(raw); !errors.Is(err, enrich.ErrTemplate) {
			t.Fatalf("%q: want ErrTemplate, got %v", raw, err)
		}
	}
}

func TestBodyTemplateKeepsTypes(t *testing.T) {
	body, err := enrich.BodyTemplateParser{}.Parse(map[string]any{"id": "${value.customer.id}", "n": "${value.customer.n}", "fixed": int64(1), "list": []any{"${headers.tenant}", true}})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := body.Render(event())
	want := map[string]any{"id": "c 1", "n": int64(7), "fixed": int64(1), "list": []any{"acme", true}}
	if !ok || !reflect.DeepEqual(got, want) || len(body.Fields()) != 3 {
		t.Fatalf("got %#v %v fields=%d", got, ok, len(body.Fields()))
	}
}

func TestSQLParamsRewrite(t *testing.T) {
	stmt, err := enrich.SQLParams{}.Rewrite(`SELECT tier, region::text FROM customers WHERE id = :id AND tenant = :tenant OR alt = :id AND note <> ':skip' AND "col:x" = 1;`)
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT tier, region::text FROM customers WHERE id = $1 AND tenant = $2 OR alt = $1 AND note <> ':skip' AND "col:x" = 1`
	if stmt.Text != want || !reflect.DeepEqual(stmt.Names, []string{"id", "tenant"}) {
		t.Fatalf("got %q %v", stmt.Text, stmt.Names)
	}
}

func TestSQLParamsRejects(t *testing.T) {
	cases := map[string]error{
		"DELETE FROM customers":                  enrich.ErrSQLNotSelect,
		"SELECT 1; DROP TABLE customers":         enrich.ErrSQLMultiStatement,
		"SELECT * FROM t WHERE id = $1":          enrich.ErrSQLPositional,
		"  with x as (select 1) select * from x": nil,
	}
	for sql, want := range cases {
		_, err := enrich.SQLParams{}.Rewrite(sql)
		if !errors.Is(err, want) && !(want == nil && err == nil) {
			t.Fatalf("%q: want %v got %v", sql, want, err)
		}
	}
}

func TestResponsePick(t *testing.T) {
	answer := map[string]any{"data": map[string]any{"tier": "gold", "tags": []any{"x", "y"}}}
	cases := []struct {
		raw  string
		want any
		ok   bool
	}{
		{raw: "$", want: answer, ok: true},
		{raw: "$.data.tier", want: "gold", ok: true},
		{raw: "$.data.tags[1]", want: "y", ok: true},
		{raw: "$.data.none", ok: false},
	}
	for _, c := range cases {
		p, err := enrich.PickParser{}.Parse(c.raw)
		if err != nil {
			t.Fatalf("%q: %v", c.raw, err)
		}
		got, ok := p.Pick(answer)
		if ok != c.ok || (ok && !reflect.DeepEqual(got, c.want)) {
			t.Fatalf("%q: %#v %v", c.raw, got, ok)
		}
	}
	if _, err := (enrich.PickParser{}).Parse("data.tier"); !errors.Is(err, enrich.ErrPick) {
		t.Fatalf("want ErrPick, got %v", err)
	}
	arr, _ := enrich.PickParser{}.Parse("$[0]")
	if v, ok := arr.Pick([]any{"first"}); !ok || v != "first" {
		t.Fatalf("$[0] on array: %v %v", v, ok)
	}
}
