package dlq

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/codec/codectest"
)

const goldenAvroSchema = `{"type":"record","name":"Order","fields":[{"name":"orderId","type":"string"},
{"name":"amount","type":"double"},{"name":"items","type":{"type":"array","items":"string"}},{"name":"note","type":["null","string"],"default":null}]}`

const goldenProto = `syntax = "proto3";
message Order { string order_id = 1; int64 cents = 2; repeated string tags = 3; bool paid = 4; }`

// schemaRecord encodes value with a registry codec of format on subject and wraps it as dead letter i.
func schemaRecord(t *testing.T, reg *codectest.MemoryRegistry, format config.Format, value any, i int) record.Record {
	t.Helper()
	c, err := codec.DefaultFactoryManager().NewSpec(codec.CodecSpec{Format: format, Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := c.Encode(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	r := failure(i, record.ErrorDecode)
	r.Ref = Ref{Topic: testDLQTopic, Partition: 0, Offset: int64(i)}
	r.Key, r.Value = []byte("ord_1"), raw
	return r
}

func schemaDetail(t *testing.T, reg *codectest.MemoryRegistry, format config.Format, rec record.Record) []byte {
	t.Helper()
	formats := SourceFormats{1: {ConnectionID: "c1", Key: config.Format{Type: config.FormatTypeString}, Value: config.Format{Type: format.Type}}}
	route := RouteInfo{ID: "r3", Name: "Order totals"}
	d := SchemaPayloads{Codecs: codec.DefaultFactoryManager(), Registries: reg}.Decode(context.Background(), formats, rec, Presenter{}.Detail(route, rec, nil))
	out, _ := json.MarshalIndent(d, "", "  ")
	return out
}

func TestGoldenAvroDetail(t *testing.T) {
	reg := codectest.NewMemoryRegistry()
	reg.Register("orders-value", codec.SchemaTypeAvro, goldenAvroSchema)
	subject := "orders-value"
	format := config.Format{Type: config.FormatTypeAvro, Subject: &subject}
	rec := schemaRecord(t, reg, format, map[string]any{"orderId": "ord_1", "amount": 19.5, "items": []any{"a", "b"}, "note": nil}, 1)
	checkGolden(t, "avro_detail.golden.json", schemaDetail(t, reg, format, rec))
}

func TestGoldenProtobufDetail(t *testing.T) {
	reg := codectest.NewMemoryRegistry()
	reg.Register("orders-value", codec.SchemaTypeProtobuf, goldenProto)
	subject, msg := "orders-value", "Order"
	format := config.Format{Type: config.FormatTypeProtobuf, Subject: &subject, MessageType: &msg}
	rec := schemaRecord(t, reg, format, map[string]any{"order_id": "ord_1", "cents": int64(1999), "tags": []any{"gift"}, "paid": true}, 1)
	checkGolden(t, "protobuf_detail.golden.json", schemaDetail(t, reg, format, rec))
}

func TestSchemaPayloadsFallBackToPlain(t *testing.T) {
	reg := codectest.NewMemoryRegistry()
	rec := failure(1, record.ErrorDecode)
	rec.Value = []byte{0x00, 0x00, 0x00, 0x00, 0x09, 0xff}
	formats := SourceFormats{1: {ConnectionID: "c1", Value: config.Format{Type: config.FormatTypeAvro}}}
	plain := Presenter{}.Detail(RouteInfo{}, rec, nil)
	got := SchemaPayloads{Codecs: codec.DefaultFactoryManager(), Registries: reg}.Decode(context.Background(), formats, rec, plain)
	if got.Value.Encoding != plain.Value.Encoding || got.Value.SchemaID != nil {
		t.Fatalf("unknown schema id must keep the plain rendering, got %+v", got.Value)
	}
}
