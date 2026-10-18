package codec_test

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
)

var ctx = context.Background()

func TestFactoryManager(t *testing.T) {
	m := codec.DefaultFactoryManager()
	for _, f := range []config.FormatType{config.FormatTypeJSON, config.FormatTypeString, config.FormatTypeBytes, config.FormatTypeAvro, config.FormatTypeProtobuf} {
		c, err := m.New(config.Format{Type: f})
		if err != nil || c.Format() != f {
			t.Fatalf("%s: %v", f, err)
		}
	}
	if _, err := m.New(config.Format{Type: config.FormatType("xml")}); !errors.Is(err, codec.ErrUnsupportedFormat) {
		t.Fatalf("xml: %v", err)
	}
	if len(m.Formats()) != 5 {
		t.Fatal("formats")
	}
}

func TestJSONCodec(t *testing.T) {
	c := codec.JSONCodec{}
	v, err := c.Decode(ctx, []byte(`{"i":9007199254740993,"f":1.5,"e":1e2,"a":[1,"<&>"],"n":null}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"i": int64(9007199254740993), "f": 1.5, "e": 100.0, "a": []any{int64(1), "<&>"}, "n": nil}
	if !reflect.DeepEqual(v, want) {
		t.Fatalf("decode %#v", v)
	}
	out, _ := c.Encode(ctx, v)
	if string(out) != `{"a":[1,"<&>"],"e":100,"f":1.5,"i":9007199254740993,"n":null}` {
		t.Fatalf("encode %s", out)
	}
	for _, bad := range []string{`{`, `{"a":1} 2`, ``, `nul`} {
		if _, err := c.Decode(ctx, []byte(bad)); !errors.Is(err, codec.ErrMalformed) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if v, err := c.Decode(ctx, nil); v != nil || err != nil {
		t.Fatal("nil bytes")
	}
	if _, err := c.Encode(ctx, func() {}); !errors.Is(err, codec.ErrUnencodable) {
		t.Fatal("func must not encode")
	}
}

func TestStringAndBytesCodecs(t *testing.T) {
	s := codec.StringCodec{}
	if v, _ := s.Decode(ctx, []byte("héllo")); v != "héllo" {
		t.Fatal(v)
	}
	if _, err := s.Decode(ctx, []byte{0xff}); !errors.Is(err, codec.ErrMalformed) {
		t.Fatal("invalid utf8")
	}
	for in, want := range map[any]string{"x": "x", int64(5): "5", 2.5: "2.5", true: "true"} {
		if b, err := s.Encode(ctx, in); err != nil || string(b) != want {
			t.Fatalf("%v: %s %v", in, b, err)
		}
	}
	if _, err := s.Encode(ctx, map[string]any{}); !errors.Is(err, codec.ErrUnencodable) {
		t.Fatal("map as string")
	}
	b := codec.BytesCodec{}
	raw := []byte{0, 1, 0xff}
	v, _ := b.Decode(ctx, raw)
	raw[0] = 9
	if !bytes.Equal(v.([]byte), []byte{0, 1, 0xff}) {
		t.Fatal("bytes must be copied")
	}
	if out, _ := b.Encode(ctx, v); !bytes.Equal(out, []byte{0, 1, 0xff}) {
		t.Fatal("bytes encode")
	}
	if out, err := b.Encode(ctx, nil); out != nil || err != nil {
		t.Fatal("nil")
	}
}

func FuzzJSONCodec(f *testing.F) {
	for _, s := range []string{`{}`, `{"a":[1,2,{"b":null}]}`, `"\ud800"`, `1e400`, `[`, "\xff", `{"a":1}{`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		c := codec.JSONCodec{}
		v, err := c.Decode(ctx, raw)
		if err != nil {
			if !errors.Is(err, codec.ErrMalformed) {
				t.Fatalf("untyped error %v", err)
			}
			return
		}
		out, err := c.Encode(ctx, v)
		if err != nil {
			t.Fatalf("re-encode: %v", err)
		}
		if _, err := c.Decode(ctx, out); err != nil {
			t.Fatalf("round trip: %v", err)
		}
	})
}
