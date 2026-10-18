package codec

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/eventrouter/router/pkg/config"
)

// JSONFactory builds JSONCodecs.
type JSONFactory struct{}

func (JSONFactory) Format() config.FormatType { return config.FormatTypeJSON }

func (JSONFactory) New(CodecSpec) (Codec, error) { return JSONCodec{}, nil }

// JSONCodec decodes objects to map[string]any; integers become int64 when exact, else float64.
type JSONCodec struct{}

func (JSONCodec) Format() config.FormatType { return config.FormatTypeJSON }

// Decode parses one JSON value and rejects trailing data.
func (JSONCodec) Decode(_ context.Context, raw []byte) (any, error) {
	if raw == nil {
		return nil, nil
	}
	v, err := TreeReader{}.Read(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return v, nil
}

// Encode writes compact JSON with sorted keys and without HTML escaping.
func (JSONCodec) Encode(_ context.Context, v any) ([]byte, error) {
	b, err := TreeWriter{}.Write(v)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnencodable, err)
	}
	return b, nil
}

// NumberPolicy converts json.Number: exact integers to int64, finite numbers to float64;
// numbers outside float64 range stay json.Number so they re-encode unchanged.
type NumberPolicy struct{}

// Normalize rewrites numbers in place throughout the tree.
func (p NumberPolicy) Normalize(v any) any {
	switch t := v.(type) {
	case json.Number:
		return p.number(t)
	case map[string]any:
		for k, e := range t {
			t[k] = p.Normalize(e)
		}
	case []any:
		for i, e := range t {
			t[i] = p.Normalize(e)
		}
	}
	return v
}

// Normalized returns a converted deep copy, leaving v untouched (document constants).
func (p NumberPolicy) Normalized(v any) any {
	switch t := v.(type) {
	case json.Number:
		return p.number(t)
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, e := range t {
			m[k] = p.Normalized(e)
		}
		return m
	case []any:
		s := make([]any, len(t))
		for i, e := range t {
			s[i] = p.Normalized(e)
		}
		return s
	}
	return v
}

func (NumberPolicy) number(n json.Number) any {
	if i, err := strconv.ParseInt(string(n), 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(string(n), 64); err == nil {
		return f
	}
	return n
}
