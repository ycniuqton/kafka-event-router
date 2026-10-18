package boundary

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestToGuestShape(t *testing.T) {
	ev := GuestEvent{Key: "k", Value: map[string]any{"a": 1.0}, Topic: "orders", Partition: 3, Offset: 9,
		Timestamp: time.Date(2024, 1, 2, 3, 4, 5, 6e6, time.UTC),
		Headers:   []Header{{Key: "trace", Value: []byte("abc")}, {Key: "bin", Value: []byte{0xff, 0x00}}, {Key: "trace", Value: []byte("last")}}}
	raw, err := JSONMarshaller{}.ToGuest(ev)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	want := map[string]any{
		"key": "k", "value": map[string]any{"a": 1.0}, "topic": "orders", "partition": 3.0, "offset": 9.0,
		"timestamp": "2024-01-02T03:04:05.006Z",
		"headers":   map[string]any{"trace": "last", "bin": map[string]any{"$bytes": "/wA="}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("guest event = %v\nwant %v", got, want)
	}
}

func TestFromGuestTable(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		outputs int
		dropped bool
		invalid bool
	}{
		{"null drops", `null`, 0, true, false},
		{"object", `{"value":{"a":1}}`, 1, false, false},
		{"array splits", `[{"value":1},{"value":2}]`, 2, false, false},
		{"empty array drops", `[]`, 0, true, false},
		{"number", `5`, 0, false, true},
		{"string", `"x"`, 0, false, true},
		{"bool", `true`, 0, false, true},
		{"array with scalar", `[{"value":1},3]`, 0, false, true},
		{"headers not object", `{"headers":["a"]}`, 0, false, true},
		{"header number", `{"headers":{"a":1}}`, 0, false, true},
		{"bad base64", `{"headers":{"a":{"$bytes":"!!"}}}`, 0, false, true},
		{"malformed", `{`, 0, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := JSONMarshaller{}.FromGuest([]byte(tc.raw))
			var oe *OutputError
			if errors.As(err, &oe) != tc.invalid {
				t.Fatalf("err = %v, invalid want %v", err, tc.invalid)
			}
			if !tc.invalid && (len(d.Outputs) != tc.outputs || d.Dropped != tc.dropped) {
				t.Fatalf("decoded = %+v", d)
			}
		})
	}
}

func TestFromGuestFields(t *testing.T) {
	d, err := JSONMarshaller{}.FromGuest([]byte(`{"key":null,"value":{"x":1},"topic":"ignored","headers":{"b":{"$bytes":"/wA="},"a":"t"}}`))
	if err != nil {
		t.Fatal(err)
	}
	o := d.Outputs[0]
	if !o.KeySet || o.Key != nil || !o.HeadersSet {
		t.Fatalf("output = %+v", o)
	}
	want := []Header{{Key: "a", Value: []byte("t")}, {Key: "b", Value: []byte{0xff, 0x00}}}
	if !reflect.DeepEqual(o.Headers, want) {
		t.Fatalf("headers = %v", o.Headers)
	}
	d, _ = JSONMarshaller{}.FromGuest([]byte(`{"value":1}`))
	if d.Outputs[0].KeySet || d.Outputs[0].HeadersSet {
		t.Fatal("absent key/headers must not be marked as set")
	}
}

func TestRoundTripHeaders(t *testing.T) {
	in := GuestEvent{Headers: []Header{{Key: "bin", Value: []byte{0xc3, 0x28}}, {Key: "txt", Value: []byte("ok")}}}
	raw, _ := JSONMarshaller{}.ToGuest(in)
	var doc map[string]json.RawMessage
	_ = json.Unmarshal(raw, &doc)
	d, err := JSONMarshaller{}.FromGuest([]byte(`{"headers":` + string(doc["headers"]) + `}`))
	if err != nil || !reflect.DeepEqual(d.Outputs[0].Headers, in.Headers) {
		t.Fatalf("round trip = %v, %v", d.Outputs, err)
	}
}
