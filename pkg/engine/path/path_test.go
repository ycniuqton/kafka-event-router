package path_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/path"
)

func TestParse(t *testing.T) {
	ok := map[string]path.FieldPath{
		"value":                      {Root: path.RootValue, Segments: []path.Segment{}},
		"value.order.items[0].price": {Root: path.RootValue, Segments: []path.Segment{{Name: "order"}, {Name: "items"}, {Index: 0, IsIndex: true}, {Name: "price"}}},
		"headers.traceId":            {Root: path.RootHeaders, Segments: []path.Segment{{Name: "traceId"}}},
		"key.id":                     {Root: path.RootKey, Segments: []path.Segment{{Name: "id"}}},
		"value[12]":                  {Root: path.RootValue, Segments: []path.Segment{{Index: 12, IsIndex: true}}},
		"topic":                      {Root: path.RootTopic, Segments: []path.Segment{}},
	}
	for in, want := range ok {
		got, err := path.Parser{}.Parse(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got.Root != want.Root || !reflect.DeepEqual(got.Segments, want.Segments) || got.String() != in {
			t.Errorf("%s: got %+v", in, got)
		}
	}
	bad := []string{"", "value..a", "value.items[x]", "value.items[-1]", "value.items[01]", "value.a[", "payload.a",
		"topic.x", "headers", "headers.a.b", "headers[0]", "value.", "value.a]", ".value"}
	for _, in := range bad {
		if _, err := (path.Parser{}).Parse(in); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

func event() *engine.Event {
	return &engine.Event{
		Key:     map[string]any{"id": "k1"},
		Value:   map[string]any{"order": map[string]any{"items": []any{map[string]any{"price": 10.5}, "x"}}, "n": nil},
		Headers: []engine.Header{{Key: "traceId", Value: []byte("t1")}, {Key: "traceId", Value: []byte("t2")}},
		Topic:   "orders", Partition: 3, Offset: 42, Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestAccessorGet(t *testing.T) {
	p := path.Parser{}
	cases := []struct {
		path   string
		want   any
		exists bool
	}{
		{"value.order.items[0].price", 10.5, true},
		{"value.order.items[1]", "x", true},
		{"value.order.items[2]", nil, false},
		{"value.order.items[1].price", nil, false},
		{"value.n", nil, true},
		{"value.missing", nil, false},
		{"key.id", "k1", true},
		{"headers.traceId", "t1", true},
		{"headers.nope", nil, false},
		{"topic", "orders", true},
		{"partition", int64(3), true},
		{"offset", int64(42), true},
		{"timestamp", "2026-01-01T00:00:00Z", true},
	}
	for _, c := range cases {
		got, ok := p.MustParse(c.path).Get(event())
		if ok != c.exists || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v,%v want %v,%v", c.path, got, ok, c.want, c.exists)
		}
	}
	if (path.Parser{}).MustParse("value").Exists(&engine.Event{}) {
		t.Error("nil value must not exist")
	}
}

func TestAccessorSetDelete(t *testing.T) {
	p := path.Parser{}
	ev := event()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(p.MustParse("value.a.b.c").Set(ev, 1))
	must(p.MustParse("value.order.items[1]").Set(ev, "y"))
	must(p.MustParse("headers.traceId").Set(ev, 7))
	must(p.MustParse("key").Set(ev, "new-key"))
	if err := p.MustParse("topic").Set(ev, "x"); err == nil {
		t.Fatal("topic is read-only")
	}
	if err := p.MustParse("value.order.items[5]").Set(ev, 1); err == nil {
		t.Fatal("out of range set")
	}
	if err := p.MustParse("value.order.items[0].price.x").Set(ev, 1); err == nil {
		t.Fatal("write through scalar")
	}
	if v, _ := p.MustParse("value.a.b.c").Get(ev); v != 1 {
		t.Fatalf("got %v", v)
	}
	if len(ev.Headers) != 1 || string(ev.Headers[0].Value) != "7" || ev.Key != "new-key" {
		t.Fatalf("headers %v key %v", ev.Headers, ev.Key)
	}
	if !p.MustParse("value.order.items[0]").Delete(ev) {
		t.Fatal("delete index")
	}
	if v, _ := p.MustParse("value.order.items").Get(ev); !reflect.DeepEqual(v, []any{"y"}) {
		t.Fatalf("after delete %v", v)
	}
	if p.MustParse("value.nothing.here").Delete(ev) || p.MustParse("value.order.items[9]").Delete(ev) {
		t.Fatal("delete of missing must report false")
	}
	if !p.MustParse("headers.traceId").Delete(ev) || len(ev.Headers) != 0 {
		t.Fatal("delete header")
	}
	if !p.MustParse("value").Delete(ev) || ev.Value != nil {
		t.Fatal("delete root")
	}
	empty := &engine.Event{}
	must(p.MustParse("value.x").Set(empty, true))
	if !reflect.DeepEqual(empty.Value, map[string]any{"x": true}) {
		t.Fatalf("created %v", empty.Value)
	}
}

func FuzzFieldPath(f *testing.F) {
	for _, s := range []string{"value.a[0].b", "headers.x", "value..", "key[1][2]", "value.a[99999999999999999999]", "timestamp"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, err := path.Parser{}.Parse(s)
		if err != nil {
			return
		}
		ev := event()
		p.Get(ev)
		_ = p.Set(ev, "v")
		p.Delete(ev)
		again, err := path.Parser{}.Parse(p.String())
		if err != nil || !reflect.DeepEqual(again, p) {
			t.Fatalf("reparse %q: %v", s, err)
		}
	})
}
