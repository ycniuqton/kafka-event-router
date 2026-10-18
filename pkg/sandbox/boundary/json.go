package boundary

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"unicode/utf8"
)

// TimeLayout is a Go time layout used at the JS boundary.
type TimeLayout string

// TimestampLayout renders event.timestamp like JavaScript Date#toISOString.
const TimestampLayout TimeLayout = "2006-01-02T15:04:05.000Z"

// JSONMarshaller is the MVP boundary: plain JSON, headers as {name: string}, $bytes for non-UTF-8.
type JSONMarshaller struct{}

func (JSONMarshaller) Kind() MarshallerKind { return MarshallerJSON }

// ToGuest renders the guest event JSON; duplicate header names keep the last value.
func (JSONMarshaller) ToGuest(ev GuestEvent) ([]byte, error) {
	return eventWriter{values: plainValues{}}.write(ev)
}

// eventWriter renders the guest event; values convert key and value members.
type eventWriter struct {
	values ValueCodec
}

func (w eventWriter) write(ev GuestEvent) ([]byte, error) {
	doc := map[Field]any{
		FieldKey:       w.values.ToGuest(ev.Key),
		FieldValue:     w.values.ToGuest(ev.Value),
		FieldHeaders:   w.headersToGuest(ev.Headers),
		FieldTopic:     ev.Topic,
		FieldPartition: ev.Partition,
		FieldOffset:    ev.Offset,
		FieldTimestamp: ev.Timestamp.UTC().Format(string(TimestampLayout)),
	}
	return json.Marshal(doc)
}

func (eventWriter) headersToGuest(hs []Header) map[string]any {
	out := make(map[string]any, len(hs))
	for _, h := range hs {
		if utf8.Valid(h.Value) {
			out[h.Key] = string(h.Value)
			continue
		}
		out[h.Key] = map[Field]string{FieldBytes: base64.StdEncoding.EncodeToString(h.Value)}
	}
	return out
}

// FromGuest validates a transform result: null drops, an object is one output, an array splits.
func (JSONMarshaller) FromGuest(raw []byte) (Decoded, error) {
	return resultReader{values: plainValues{}}.read(raw)
}

// ValueCodec converts key/value members between Go values and their guest JSON form.
type ValueCodec interface {
	ToGuest(v any) any
	FromGuest(v any) (any, error)
	UseNumber() bool
}

// plainValues passes values through unchanged (JSON-format routes).
type plainValues struct{}

func (plainValues) ToGuest(v any) any            { return v }
func (plainValues) FromGuest(v any) (any, error) { return v, nil }
func (plainValues) UseNumber() bool              { return false }

// resultReader enforces the transform output contract; values convert key and value members.
type resultReader struct {
	values ValueCodec
}

func (r resultReader) read(raw []byte) (Decoded, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	if r.values.UseNumber() {
		dec.UseNumber()
	}
	if err := dec.Decode(&v); err != nil {
		return Decoded{}, &OutputError{Reason: err.Error()}
	}
	switch t := v.(type) {
	case nil:
		return Decoded{Dropped: true}, nil
	case map[string]any:
		out, err := r.output(t)
		return Decoded{Outputs: []Output{out}}, err
	case []any:
		return r.outputs(t)
	}
	return Decoded{}, &OutputError{Reason: fmt.Sprintf(string(ReasonNotObject), JSType{}.Of(v))}
}

func (r resultReader) outputs(items []any) (Decoded, error) {
	d := Decoded{Outputs: make([]Output, 0, len(items))}
	for i, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return Decoded{}, &OutputError{Reason: fmt.Sprintf(string(ReasonItemNotObject), i)}
		}
		out, err := r.output(obj)
		if err != nil {
			return Decoded{}, err
		}
		d.Outputs = append(d.Outputs, out)
	}
	d.Dropped = len(d.Outputs) == 0
	return d, nil
}

func (r resultReader) output(obj map[string]any) (Output, error) {
	value, err := r.values.FromGuest(obj[string(FieldValue)])
	if err != nil {
		return Output{}, err
	}
	out := Output{Value: value}
	if key, ok := obj[string(FieldKey)]; ok {
		out.KeySet = true
		if out.Key, err = r.values.FromGuest(key); err != nil {
			return Output{}, err
		}
	}
	raw, ok := obj[string(FieldHeaders)]
	if !ok || raw == nil {
		return out, nil
	}
	hs, err := r.headersFromGuest(raw)
	out.Headers, out.HeadersSet = hs, err == nil
	return out, err
}

func (resultReader) headersFromGuest(raw any) ([]Header, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, &OutputError{Reason: string(ReasonBadHeaders)}
	}
	names := make([]string, 0, len(obj))
	for k := range obj {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]Header, 0, len(names))
	for _, k := range names {
		b, err := HeaderValue{}.Decode(k, obj[k])
		if err != nil {
			return nil, err
		}
		out = append(out, Header{Key: k, Value: b})
	}
	return out, nil
}

// HeaderValue decodes one guest header value.
type HeaderValue struct{}

// Decode accepts a string or {"$bytes": base64}.
func (HeaderValue) Decode(name string, v any) ([]byte, error) {
	if s, ok := v.(string); ok {
		return []byte(s), nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, &OutputError{Reason: fmt.Sprintf(string(ReasonBadHeader), name)}
	}
	enc, ok := obj[string(FieldBytes)].(string)
	if !ok {
		return nil, &OutputError{Reason: fmt.Sprintf(string(ReasonBadHeader), name)}
	}
	b, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, &OutputError{Reason: fmt.Sprintf(string(ReasonBadHeader), name)}
	}
	return b, nil
}

// JSTypeName is the JS type of a decoded JSON value, used in error messages.
type JSTypeName string

const (
	TypeString  JSTypeName = "string"
	TypeNumber  JSTypeName = "number"
	TypeBoolean JSTypeName = "boolean"
	TypeOther   JSTypeName = "value"
)

// JSType names the JS type of a decoded JSON value.
type JSType struct{}

// Of returns the type name of v.
func (JSType) Of(v any) JSTypeName {
	switch v.(type) {
	case string:
		return TypeString
	case float64:
		return TypeNumber
	case bool:
		return TypeBoolean
	}
	return TypeOther
}
