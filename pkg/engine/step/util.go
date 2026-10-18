package step

import (
	"time"

	"github.com/eventrouter/router/pkg/engine/codec"
)

// NumberNormalizer converts document constants (json.Number) to engine number types.
type NumberNormalizer struct{}

// Of returns a normalised deep copy; the input is not modified.
func (NumberNormalizer) Of(v any) any { return codec.NumberPolicy{}.Normalized(v) }

// TimeText renders times as RFC 3339 with nanoseconds in UTC.
type TimeText struct{}

// Of formats t.
func (TimeText) Of(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
