package codec_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/codec/codectest"
)

const commonProto = `syntax = "proto3";
package shop.common;
message Money { string currency = 1; int64 cents = 2; }`

const orderProto = `syntax = "proto3";
package shop;
import "google/protobuf/timestamp.proto";
import "common.proto";
message Ignored { string x = 1; }
message Order {
  enum Status { NEW = 0; PAID = 1; }
  message Line { string sku = 1; uint32 qty = 2; }
  string id = 1;
  int32 count = 2;
  int64 big = 3;
  double amount = 4;
  float ratio = 5;
  bool paid = 6;
  bytes raw = 7;
  Status status = 8;
  repeated Line lines = 9;
  map<string, int64> attrs = 10;
  google.protobuf.Timestamp at = 11;
  shop.common.Money total = 12;
  optional string note = 13;
  repeated string tags = 14;
  map<int32, string> codes = 15;
}`

func protoRegistry() *codectest.MemoryRegistry {
	reg := codectest.NewMemoryRegistry()
	reg.Register("common", codec.SchemaTypeProtobuf, commonProto)
	reg.Register("orders-value", codec.SchemaTypeProtobuf, orderProto, codec.SchemaReference{Name: "common.proto", Subject: "common", Version: 1})
	return reg
}

func protoCodec(t testing.TB, reg codec.SchemaRegistry, subject, messageType string) codec.Codec {
	t.Helper()
	spec := codec.CodecSpec{Format: config.Format{Type: config.FormatTypeProtobuf, Subject: &subject}, Registry: reg}
	if messageType != "" {
		spec.Format.MessageType = &messageType
	}
	c, err := codec.DefaultFactoryManager().NewSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func protoOrder() map[string]any {
	return map[string]any{
		"id": "o-1", "count": int64(2), "big": int64(1) << 40, "amount": 9.5, "ratio": 0.5, "paid": true,
		"raw": []byte{1, 2}, "status": "PAID",
		"lines": []any{map[string]any{"sku": "A", "qty": int64(1)}, map[string]any{"sku": "B", "qty": int64(0)}},
		"attrs": map[string]any{"k": int64(5)}, "at": "2026-01-02T03:04:05.5Z",
		"total": map[string]any{"currency": "EUR", "cents": int64(1999)}, "note": "gift", "tags": []any{"x"},
		"codes": map[string]any{"7": "seven"},
	}
}

func TestProtobufRoundTrip(t *testing.T) {
	reg := protoRegistry()
	c := protoCodec(t, reg, "orders-value", "shop.Order")
	raw, err := c.Encode(context.Background(), protoOrder())
	if err != nil {
		t.Fatal(err)
	}
	frame, _ := codec.Framing{}.Parse(raw)
	idx, _, err := codec.MessageIndexes{}.Read(frame.Body)
	if err != nil || !reflect.DeepEqual(idx, []int{1}) {
		t.Fatalf("indexes %v %v", idx, err)
	}
	got, err := c.Decode(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, protoOrder()) {
		t.Fatalf("round trip\n got %#v\nwant %#v", got, protoOrder())
	}
}

func TestProtobufDefaultsAndNested(t *testing.T) {
	reg := protoRegistry()
	c := protoCodec(t, reg, "orders-value", "Line")
	raw, err := c.Encode(context.Background(), map[string]any{"sku": "Z", "unknown": 1})
	if err != nil {
		t.Fatal(err)
	}
	frame, _ := codec.Framing{}.Parse(raw)
	idx, _, _ := codec.MessageIndexes{}.Read(frame.Body)
	if !reflect.DeepEqual(idx, []int{1, 0}) {
		t.Fatalf("nested indexes %v", idx)
	}
	got, _ := c.Decode(context.Background(), raw)
	if !reflect.DeepEqual(got, map[string]any{"sku": "Z", "qty": int64(0)}) {
		t.Fatalf("defaults %#v", got)
	}
	first := protoCodec(t, reg, "orders-value", "")
	raw, _ = first.Encode(context.Background(), map[string]any{"x": "y"})
	frame, _ = codec.Framing{}.Parse(raw)
	if frame.Body[0] != 0 {
		t.Fatal("first message uses the single 0 index byte")
	}
	empty, _ := c.Decode(context.Background(), codec.Framing{}.Write(frame.SchemaID, []byte{0}))
	if !reflect.DeepEqual(empty, map[string]any{"x": ""}) {
		t.Fatalf("empty first message %#v", empty)
	}
}

func TestProtobufErrors(t *testing.T) {
	reg := protoRegistry()
	c := protoCodec(t, reg, "orders-value", "shop.Order")
	ctx := context.Background()
	bad := map[string]map[string]any{
		"type":      {"id": 5},
		"overflow":  {"count": int64(1) << 40},
		"enum":      {"status": "LOST"},
		"list":      {"lines": "x"},
		"unsigned":  {"lines": []any{map[string]any{"qty": int64(-1)}}},
		"timestamp": {"at": true},
	}
	for name, v := range bad {
		if _, err := c.Encode(ctx, v); !errors.Is(err, codec.ErrUnencodable) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := protoCodec(t, reg, "orders-value", "Nope").Encode(ctx, map[string]any{}); !errors.Is(err, codec.ErrUnencodable) {
		t.Errorf("unknown message type: %v", err)
	}
	if _, err := c.Decode(ctx, codec.Framing{}.Write(2, []byte{0x08})); !errors.Is(err, codec.ErrMalformed) {
		t.Errorf("index out of range: %v", err)
	}
	if _, err := c.Decode(ctx, codec.Framing{}.Write(2, []byte{0x02, 0x02, 0xff})); !errors.Is(err, codec.ErrMalformed) {
		t.Errorf("bad body: %v", err)
	}
	avroID := reg.Register("avro", codec.SchemaTypeAvro, `"string"`)
	if _, err := c.Decode(ctx, codec.Framing{}.Write(avroID, []byte{0})); !errors.Is(err, codec.ErrSchemaType) {
		t.Errorf("wrong schema type: %v", err)
	}
	broken := reg.Register("broken", codec.SchemaTypeProtobuf, `syntax = "proto3"; message {`)
	if _, err := c.Decode(ctx, codec.Framing{}.Write(broken, []byte{0})); !errors.Is(err, codec.ErrMalformed) {
		t.Errorf("broken schema: %v", err)
	}
}

func TestMessageIndexes(t *testing.T) {
	for _, idx := range [][]int{{0}, {1}, {0, 2}, {3, 1, 4}} {
		b := codec.MessageIndexes{}.Write(idx)
		got, rest, err := codec.MessageIndexes{}.Read(append(b, 0xAA))
		if err != nil || !reflect.DeepEqual(got, idx) || len(rest) != 1 {
			t.Fatalf("%v: %v %v %v", idx, got, rest, err)
		}
	}
	if _, _, err := (codec.MessageIndexes{}).Read([]byte{0x80}); err == nil {
		t.Fatal("truncated varint")
	}
}

func FuzzProtobufDecode(f *testing.F) {
	reg := protoRegistry()
	c := protoCodec(f, reg, "orders-value", "shop.Order")
	seed, _ := c.Encode(context.Background(), protoOrder())
	f.Add(seed)
	f.Add([]byte{0, 0, 0, 0, 2, 0})
	f.Fuzz(func(t *testing.T, raw []byte) {
		if _, err := c.Decode(context.Background(), raw); err != nil && !errors.Is(err, codec.ErrMalformed) && !errors.Is(err, codec.ErrSchemaNotFound) && !errors.Is(err, codec.ErrSchemaType) {
			t.Fatalf("untyped error %v", err)
		}
	})
}
