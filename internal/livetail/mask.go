package livetail

import (
	"context"

	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/rbac"
)

// TailMasker hides payload values of a record before it reaches a socket.
type TailMasker interface {
	Mask(rec TailRecord) TailRecord
}

// TypePreservingTailMasker masks keys, values and header values with live-preview's type-preserving
// masker (strings → masked text, numbers → 0); topics, offsets, outcome and error texts stay readable.
type TypePreservingTailMasker struct {
	Values preview.ValueMasker
}

// NewTypePreservingTailMasker uses the default masker of live-preview's MaskerManager.
func NewTypePreservingTailMasker() TypePreservingTailMasker {
	return TypePreservingTailMasker{Values: preview.DefaultMaskerManager().For(preview.MaskTypePreserving)}
}

// Mask implements TailMasker.
func (m TypePreservingTailMasker) Mask(rec TailRecord) TailRecord {
	rec.Input = m.event(rec.Input)
	outs := make([]TailEvent, 0, len(rec.Outputs))
	for _, o := range rec.Outputs {
		outs = append(outs, m.event(o))
	}
	rec.Outputs = outs
	return rec
}

// event masks one event's key, value and headers.
func (m TypePreservingTailMasker) event(e TailEvent) TailEvent {
	e.Value = m.value(e.Value, e.Encoding)
	if e.Key != nil {
		e.Key = m.Values.Mask(e.Key)
	}
	headers := make([]Header, 0, len(e.Headers))
	for _, h := range e.Headers {
		headers = append(headers, Header{Key: h.Key, Value: m.Values.MaskText(h.Value)})
	}
	if len(headers) > 0 {
		e.Headers = headers
	}
	return e
}

// value masks a JSON tree leaf by leaf and any other payload as one text.
func (m TypePreservingTailMasker) value(v any, enc Encoding) any {
	if enc == EncodingJSON {
		return m.Values.Mask(v)
	}
	text, _ := v.(string)
	return m.Values.MaskText(text)
}

// VisibilityDecider tells whether the caller sees a route's tail masked (decision 5: Viewers always,
// other roles on sensitive connections).
type VisibilityDecider interface {
	Masked(ctx context.Context, sourceConnectionID string) bool
}

// RBACVisibility adapts access-control's SampleAccess port (surface livetail); errors fail closed.
type RBACVisibility struct {
	Access rbac.SampleAccess
}

// Masked implements VisibilityDecider.
func (v RBACVisibility) Masked(ctx context.Context, sourceConnectionID string) bool {
	d, err := v.Access.Decide(ctx, rbac.SurfaceLiveTail, sourceConnectionID)
	return err != nil || d.Masked
}

// AlwaysMasked masks every tail (fail-closed fallback when access control is not wired).
type AlwaysMasked struct{}

// Masked implements VisibilityDecider.
func (AlwaysMasked) Masked(context.Context, string) bool { return true }
