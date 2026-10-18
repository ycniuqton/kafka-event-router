package codec_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/codec/codectest"
)

const orderSchema = `{"type":"record","name":"Order","namespace":"shop","fields":[
 {"name":"id","type":"string"},
 {"name":"qty","type":"int"},
 {"name":"amount","type":"double"},
 {"name":"total","type":"long"},
 {"name":"paid","type":"boolean"},
 {"name":"note","type":["null","string"],"default":null},
 {"name":"ref","type":["null","long","string"],"default":null},
 {"name":"at","type":{"type":"long","logicalType":"timestamp-millis"}},
 {"name":"day","type":{"type":"int","logicalType":"date"}},
 {"name":"price","type":{"type":"bytes","logicalType":"decimal","precision":10,"scale":2}},
 {"name":"status","type":{"type":"enum","name":"Status","symbols":["NEW","PAID"]}},
 {"name":"tags","type":{"type":"array","items":"string"}},
 {"name":"attrs","type":{"type":"map","values":"long"}},
 {"name":"hash","type":{"type":"fixed","name":"H2","size":2}},
 {"name":"customer","type":["null",{"type":"record","name":"Customer","fields":[{"name":"name","type":"string"}]}],"default":null},
 {"name":"source","type":"string","default":"web"},
 {"name":"raw","type":"bytes"}
]}`

func orderValue() map[string]any {
	return map[string]any{
		"id": "o-1", "qty": int64(2), "amount": 9.5, "total": int64(1900), "paid": true, "note": "gift", "ref": int64(7),
		"at": "2026-01-02T03:04:05.123Z", "day": "2026-01-02", "price": "123.45", "status": "PAID",
		"tags": []any{"a", "b"}, "attrs": map[string]any{"k": int64(1)}, "hash": []byte("ab"),
		"customer": map[string]any{"name": "Ada"}, "source": "app", "raw": []byte{0, 1},
	}
}

func avroCodec(t *testing.T, reg codec.SchemaRegistry, subject string) codec.Codec {
	t.Helper()
	c, err := codec.DefaultFactoryManager().NewSpec(codec.CodecSpec{Format: config.Format{Type: config.FormatTypeAvro, Subject: &subject}, Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAvroRoundTrip(t *testing.T) {
	reg := codectest.NewMemoryRegistry()
	id := reg.Register("orders-value", codec.SchemaTypeAvro, orderSchema)
	c := avroCodec(t, reg, "orders-value")
	raw, err := c.Encode(context.Background(), orderValue())
	if err != nil {
		t.Fatal(err)
	}
	if raw[0] != 0 {
		t.Fatal("not framed")
	}
	frame, _ := codec.Framing{}.Parse(raw)
	if frame.SchemaID != id {
		t.Fatalf("schema id %d", frame.SchemaID)
	}
	got, err := c.Decode(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, orderValue()) {
		t.Fatalf("round trip\n got %#v\nwant %#v", got, orderValue())
	}
}

func TestAvroDefaultsAndNullableUnion(t *testing.T) {
	reg := codectest.NewMemoryRegistry()
	reg.Register("orders-value", codec.SchemaTypeAvro, orderSchema)
	c := avroCodec(t, reg, "orders-value")
	v := orderValue()
	delete(v, "note")
	delete(v, "source")
	v["customer"] = nil
	v["ref"] = "text-ref"
	raw, err := c.Encode(context.Background(), v)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := c.Decode(context.Background(), raw)
	m := got.(map[string]any)
	if m["note"] != nil || m["source"] != "web" || m["customer"] != nil || m["ref"] != "text-ref" {
		t.Fatalf("defaults/unions: %#v", m)
	}
}

func TestAvroEncodeErrors(t *testing.T) {
	reg := codectest.NewMemoryRegistry()
	reg.Register("orders-value", codec.SchemaTypeAvro, orderSchema)
	c := avroCodec(t, reg, "orders-value")
	cases := map[string]func(m map[string]any){
		"wrong type":   func(m map[string]any) { m["qty"] = "two" },
		"int overflow": func(m map[string]any) { m["qty"] = int64(1) << 40 },
		"bad enum":     func(m map[string]any) { m["status"] = "LOST" },
		"missing":      func(m map[string]any) { delete(m, "id") },
		"fixed size":   func(m map[string]any) { m["hash"] = []byte("abc") },
		"bad decimal":  func(m map[string]any) { m["price"] = "x" },
		"not object":   func(m map[string]any) { m["customer"] = 5 },
	}
	for name, mutate := range cases {
		v := orderValue()
		mutate(v)
		if _, err := c.Encode(context.Background(), v); !errors.Is(err, codec.ErrUnencodable) {
			t.Errorf("%s: want unencodable, got %v", name, err)
		}
	}
	if _, err := avroCodec(t, reg, "").Encode(context.Background(), orderValue()); !errors.Is(err, codec.ErrSubjectRequired) {
		t.Errorf("no subject: %v", err)
	}
	if _, err := avroCodec(t, reg, "unknown").Encode(context.Background(), orderValue()); !errors.Is(err, codec.ErrSchemaNotFound) {
		t.Errorf("unknown subject: %v", err)
	}
}

func TestAvroDecodeErrors(t *testing.T) {
	reg := codectest.NewMemoryRegistry()
	reg.Register("orders-value", codec.SchemaTypeAvro, orderSchema)
	c := avroCodec(t, reg, "orders-value")
	ctx := context.Background()
	if _, err := c.Decode(ctx, []byte(`{"json":true}`)); !errors.Is(err, codec.ErrMalformed) {
		t.Errorf("unframed: %v", err)
	}
	if _, err := c.Decode(ctx, codec.Framing{}.Write(99, nil)); !errors.Is(err, codec.ErrSchemaNotFound) || errors.Is(err, codec.ErrTransient) {
		t.Errorf("unknown id: %v", err)
	}
	if _, err := c.Decode(ctx, codec.Framing{}.Write(1, []byte{0xff})); !errors.Is(err, codec.ErrMalformed) {
		t.Errorf("truncated body: %v", err)
	}
	if v, err := c.Decode(ctx, nil); v != nil || err != nil {
		t.Errorf("nil: %v %v", v, err)
	}
	reg.Down = true
	fresh := avroCodec(t, reg, "orders-value")
	if _, err := fresh.Decode(ctx, codec.Framing{}.Write(1, nil)); !errors.Is(err, codec.ErrTransient) {
		t.Errorf("registry down: %v", err)
	}
	unconfigured := avroCodec(t, nil, "orders-value")
	if _, err := unconfigured.Decode(ctx, codec.Framing{}.Write(1, nil)); !errors.Is(err, codec.ErrRegistryNotConfigured) {
		t.Errorf("unconfigured: %v", err)
	}
}

func TestAvroReferences(t *testing.T) {
	reg := codectest.NewMemoryRegistry()
	reg.Register("customer", codec.SchemaTypeAvro, `{"type":"record","name":"Customer","namespace":"crm","fields":[{"name":"name","type":"string"}]}`)
	reg.Register("orders-value", codec.SchemaTypeAvro, `{"type":"record","name":"O","namespace":"crm","fields":[{"name":"c","type":"crm.Customer"}]}`,
		codec.SchemaReference{Name: "crm.Customer", Subject: "customer", Version: 1})
	c := avroCodec(t, reg, "orders-value")
	v := map[string]any{"c": map[string]any{"name": "Ada"}}
	raw, err := c.Encode(context.Background(), v)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Decode(context.Background(), raw)
	if err != nil || !reflect.DeepEqual(got, v) {
		t.Fatalf("got %#v %v", got, err)
	}
}

func TestHTTPRegistry(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		user, pass, _ := r.BasicAuth()
		switch {
		case user != "u" || pass != "p":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/schemas/ids/7":
			_, _ = w.Write([]byte(`{"schema":"\"string\""}`))
		case r.URL.Path == "/subjects/a b/versions/latest":
			_, _ = w.Write([]byte(`{"subject":"a b","id":7,"version":3,"schema":"\"string\""}`))
		case r.URL.Path == "/schemas/ids/500":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	src := staticSource{cfg: codec.RegistryConfig{URL: srv.URL + "/", Auth: codec.RegistryAuthBasic, Username: "u", Password: "p"}}
	p := &codec.HTTPRegistryProvider{Source: src}
	reg, err := p.Registry(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s, err := reg.SchemaByID(ctx, 7)
	if err != nil || s.ID != 7 || s.EffectiveType() != codec.SchemaTypeAvro {
		t.Fatalf("by id %+v %v", s, err)
	}
	_, _ = reg.SchemaByID(ctx, 7)
	if hits.Load() != 1 {
		t.Fatalf("cache miss: %d hits", hits.Load())
	}
	if l, err := reg.Latest(ctx, "a b"); err != nil || l.Version != 3 {
		t.Fatalf("latest %+v %v", l, err)
	}
	if _, err := reg.SchemaByID(ctx, 8); !errors.Is(err, codec.ErrSchemaNotFound) {
		t.Fatalf("404: %v", err)
	}
	if _, err := reg.SchemaByID(ctx, 500); !errors.Is(err, codec.ErrTransient) {
		t.Fatalf("503: %v", err)
	}
	again, _ := p.Registry(ctx, "c1")
	if again != reg {
		t.Fatal("provider should reuse the client")
	}
	bad := codec.NewHTTPRegistry(codec.RegistryConfig{URL: "http://127.0.0.1:1"}, nil)
	if _, err := bad.SchemaByID(ctx, 1); !errors.Is(err, codec.ErrTransient) || !strings.Contains(err.Error(), "registry") {
		t.Fatalf("network: %v", err)
	}
}

type staticSource struct {
	cfg codec.RegistryConfig
}

func (s staticSource) RegistryConfig(context.Context, string) (codec.RegistryConfig, bool, error) {
	return s.cfg, true, nil
}

func FuzzAvroDecode(f *testing.F) {
	reg := codectest.NewMemoryRegistry()
	reg.Register("orders-value", codec.SchemaTypeAvro, orderSchema)
	subject := "orders-value"
	c, _ := codec.DefaultFactoryManager().NewSpec(codec.CodecSpec{Format: config.Format{Type: config.FormatTypeAvro, Subject: &subject}, Registry: reg})
	seed, _ := c.Encode(context.Background(), orderValue())
	f.Add(seed)
	f.Add([]byte{0, 0, 0, 0, 1})
	f.Add([]byte{0, 0, 0, 0, 1, 2, 'x'})
	f.Add([]byte("{}"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		v, err := c.Decode(context.Background(), raw)
		if err != nil {
			if !errors.Is(err, codec.ErrMalformed) && !errors.Is(err, codec.ErrSchemaNotFound) {
				t.Fatalf("untyped error %v", err)
			}
			return
		}
		if _, err := c.Encode(context.Background(), v); err != nil && !errors.Is(err, codec.ErrUnencodable) {
			t.Fatalf("untyped encode error: %v", err)
		}
	})
}
