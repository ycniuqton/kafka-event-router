package boundary

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"
)

func taggedRoundTrip(t *testing.T, value any) any {
	t.Helper()
	raw, err := TaggedMarshaller{}.ToGuest(GuestEvent{Value: value, Timestamp: time.Unix(0, 0)})
	if err != nil {
		t.Fatal(err)
	}
	var ev map[string]json.RawMessage
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	out := append(append([]byte(`{"value":`), ev["value"]...), '}')
	d, err := TaggedMarshaller{}.FromGuest(out)
	if err != nil {
		t.Fatal(err)
	}
	return d.Outputs[0].Value
}

func TestTaggedRoundTrip(t *testing.T) {
	ts := time.Date(2024, 2, 29, 23, 59, 59, 123456789, time.UTC)
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"int64 beyond 2^53", int64(1<<62 + 1), int64(1<<62 + 1)},
		{"negative int64 beyond 2^53", int64(math.MinInt64), int64(math.MinInt64)},
		{"max safe integer stays a number", int64(MaxSafeInteger), int64(MaxSafeInteger)},
		{"uint64 above int64", uint64(math.MaxUint64), uint64(math.MaxUint64)},
		{"big.Int", new(big.Int).Lsh(big.NewInt(1), 60), int64(1 << 60)},
		{"bytes", []byte{0, 1, 0xfe, 0xff}, []byte{0, 1, 0xfe, 0xff}},
		{"decimal", big.NewRat(-123456789, 1000), big.NewRat(-123456789, 1000)},
		{"timestamp", ts, ts},
		{"plain float", 1.5, 1.5},
		{"nested", map[string]any{"ids": []any{int64(1 << 60), "x"}, "amount": big.NewRat(5, 2)},
			map[string]any{"ids": []any{int64(1 << 60), "x"}, "amount": big.NewRat(5, 2)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := taggedRoundTrip(t, c.in)
			if r, ok := c.want.(*big.Rat); ok {
				if gr, _ := got.(*big.Rat); gr == nil || gr.Cmp(r) != 0 {
					t.Fatalf("got %#v want %v", got, r)
				}
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %#v (%T) want %#v (%T)", got, got, c.want, c.want)
			}
		})
	}
}

func TestTaggedGuestShape(t *testing.T) {
	ev := GuestEvent{Key: int64(1 << 60), Value: map[string]any{
		"n": int64(7), "big": int64(1 << 60), "b": []byte("hi"), "d": big.NewRat(1, 8),
		"t": time.Date(2024, 1, 1, 0, 0, 0, 0, time.FixedZone("x", 3600)),
	}, Timestamp: time.Unix(0, 0)}
	raw, _ := TaggedMarshaller{}.ToGuest(ev)
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	want := map[string]any{
		"n": 7.0, "big": map[string]any{"$long": "1152921504606846976"}, "b": map[string]any{"$bytes": "aGk="},
		"d": map[string]any{"$decimal": "0.125"}, "t": map[string]any{"$timestamp": "2023-12-31T23:00:00Z"},
	}
	if !reflect.DeepEqual(got["value"], want) {
		t.Fatalf("value = %v\nwant %v", got["value"], want)
	}
	if !reflect.DeepEqual(got["key"], map[string]any{"$long": "1152921504606846976"}) {
		t.Fatalf("key = %v", got["key"])
	}
}

func TestTaggedFromGuestRules(t *testing.T) {
	d, err := TaggedMarshaller{}.FromGuest([]byte(`{"key":{"$long":"9007199254740993"},"value":{"i":3,"f":2.5,"two":{"$long":"1","x":1},"num":{"$bytes":5}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Outputs[0].Key != int64(9007199254740993) {
		t.Fatalf("key %#v", d.Outputs[0].Key)
	}
	v := d.Outputs[0].Value.(map[string]any)
	if v["i"] != int64(3) || v["f"] != 2.5 {
		t.Fatalf("numbers %#v %#v", v["i"], v["f"])
	}
	if _, ok := v["two"].(map[string]any); !ok {
		t.Fatalf("object with extra members must stay an object: %#v", v["two"])
	}
	if _, ok := v["num"].(map[string]any); !ok {
		t.Fatalf("tag with non-string member must stay an object: %#v", v["num"])
	}
	for _, bad := range []string{`{"value":{"$long":"x"}}`, `{"value":{"$timestamp":"yesterday"}}`, `{"value":[{"$decimal":"1.2.3"}]}`, `{"value":{"$bytes":"%%"}}`} {
		_, err := TaggedMarshaller{}.FromGuest([]byte(bad))
		var oe *OutputError
		if !errors.As(err, &oe) {
			t.Errorf("%s: want OutputError, got %v", bad, err)
		}
	}
}

func TestDecimalText(t *testing.T) {
	cases := map[string]*big.Rat{"42": big.NewRat(42, 1), "-0.5": big.NewRat(-1, 2), "12.345": big.NewRat(12345, 1000), "0.0001": big.NewRat(1, 10000)}
	for want, r := range cases {
		if got := (DecimalText{}).Format(r); got != want {
			t.Errorf("Format(%v) = %s want %s", r, got, want)
		}
	}
	if got := (DecimalText{}).Format(big.NewRat(1, 3)); len(got) != int(DecimalFallbackScale)+2 {
		t.Errorf("non-terminating decimal = %s", got)
	}
}

func TestRegistrySelect(t *testing.T) {
	r := NewRegistry()
	cases := []struct {
		formats []Format
		want    MarshallerKind
	}{
		{nil, MarshallerJSON},
		{[]Format{FormatJSON, FormatString}, MarshallerJSON},
		{[]Format{FormatJSON, FormatAvro}, MarshallerTagged},
		{[]Format{FormatProtobuf}, MarshallerTagged},
		{[]Format{"unknown"}, MarshallerJSON},
	}
	for _, c := range cases {
		if got := r.Select(c.formats).Kind(); got != c.want {
			t.Errorf("Select(%v) = %s want %s", c.formats, got, c.want)
		}
	}
}
