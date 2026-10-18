package dlq

import (
	"context"
	"encoding/json"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// SampleVisibility decides whether the current principal sees a connection's payloads masked
// (access-control SampleVisibility: Viewers always, other roles on sensitive connections).
type SampleVisibility interface {
	Masked(ctx context.Context, connectionID string) (bool, error)
}

// RBACVisibility adapts access-control's SampleAccess port (surface dlq); errors fail closed.
type RBACVisibility struct {
	Access rbac.SampleAccess
}

// Masked implements SampleVisibility.
func (v RBACVisibility) Masked(ctx context.Context, connectionID string) (bool, error) {
	d, err := v.Access.Decide(ctx, rbac.SurfaceDLQ, connectionID)
	if err != nil {
		return true, nil
	}
	return d.Masked, nil
}

// AlwaysMasked masks every payload (tests, fail-closed fallback).
type AlwaysMasked struct{}

// Masked implements SampleVisibility.
func (AlwaysMasked) Masked(context.Context, string) (bool, error) { return true, nil }

// ValueMasker hides a decoded value while keeping its JSON type.
type ValueMasker interface {
	Mask(v any) any
}

// TypePreservingMasker masks strings with the engine's full mask, numbers to 0, booleans to false;
// objects and arrays keep their shape (decision: masks keep types).
type TypePreservingMasker struct{}

// Mask implements ValueMasker.
func (m TypePreservingMasker) Mask(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = m.Mask(x)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = m.Mask(x)
		}
		return out
	case string:
		return m.Text(t)
	case float64, json.Number:
		return 0
	case bool:
		return false
	}
	return v
}

// Text masks a string keeping its length.
func (TypePreservingMasker) Text(s string) string {
	return (ctxhelpers.FullMask{}).Apply(s, ctxhelpers.DefaultMaskPolicy())
}

// MaskingViewPolicy applies the visibility decision with a type-preserving masker (DL-106).
type MaskingViewPolicy struct {
	Visibility SampleVisibility
	Masker     ValueMasker
}

// Present implements ViewPolicy: masked views carry masked=true.
func (p MaskingViewPolicy) Present(ctx context.Context, _ Viewer, conn string, d DetailView) (DetailView, error) {
	masked, err := p.Visibility.Masked(ctx, conn)
	if err != nil || !masked {
		return d, err
	}
	pm := PayloadMasker{Masker: p.Masker}
	d.Key, d.Value = pm.Encoded(d.Key), pm.Encoded(d.Value)
	headers := make([]EncodedHeader, 0, len(d.Headers))
	for _, h := range d.Headers {
		headers = append(headers, EncodedHeader{Key: h.Key, Value: pm.Text(h.Value), Encoding: EncodingUTF8})
	}
	d.Headers, d.Masked = headers, true
	return d, nil
}

// PresentRecord implements ViewPolicy: masks the bytes of a record (Open in Studio).
func (p MaskingViewPolicy) PresentRecord(ctx context.Context, _ Viewer, conn string, rec record.Record) (record.Record, bool, error) {
	masked, err := p.Visibility.Masked(ctx, conn)
	if err != nil || !masked {
		return rec, false, err
	}
	pm := PayloadMasker{Masker: p.Masker}
	rec.Key, rec.Value = pm.Bytes(rec.Key), pm.Bytes(rec.Value)
	headers := make([]engine.Header, 0, len(rec.Headers))
	for _, h := range rec.Headers {
		headers = append(headers, engine.Header{Key: h.Key, Value: []byte(pm.Text(string(h.Value)))})
	}
	rec.Headers = headers
	return rec, true, nil
}

// PayloadMasker masks rendered values and raw bytes.
type PayloadMasker struct {
	Masker ValueMasker
}

// Encoded masks a rendered value; structured data keeps its encoding, text and raw bytes become masked text.
func (p PayloadMasker) Encoded(v EncodedValue) EncodedValue {
	if v.Data == nil {
		return v
	}
	if _, structured := StructuredEncodings[v.Encoding]; structured {
		return EncodedValue{Encoding: v.Encoding, Data: p.Masker.Mask(p.decoded(v.Data)), SchemaID: v.SchemaID}
	}
	text, _ := v.Data.(string)
	return EncodedValue{Encoding: EncodingUTF8, Data: p.Text(text)}
}

// Bytes masks raw bytes: JSON stays JSON with masked leaves, anything else becomes masked text.
func (p PayloadMasker) Bytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	var v any
	if json.Unmarshal(b, &v) == nil {
		out, _ := json.Marshal(p.Masker.Mask(v))
		return out
	}
	return []byte(p.Text(string(b)))
}

// Text masks a string.
func (p PayloadMasker) Text(s string) string {
	return TypePreservingMasker{}.Text(s)
}

// decoded turns raw JSON data into Go values.
func (PayloadMasker) decoded(data any) any {
	raw, ok := data.(json.RawMessage)
	if !ok {
		raw, _ = json.Marshal(data)
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return v
}

// StructuredEncodings are encodings whose data is a JSON tree.
var StructuredEncodings = map[Encoding]struct{}{EncodingJSON: {}, EncodingAvro: {}, EncodingProtobuf: {}}
