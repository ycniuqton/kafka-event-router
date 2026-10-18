package boundary

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"time"
)

// Tag is the base of tagged boundary values: one "$name" member holding a string.
type Tag interface {
	Field() Field
	Decode(text string) (any, error)
}

// TagReason is an OutputError message for a malformed tagged value.
type TagReason string

const ReasonBadTag TagReason = "%s value %q is not valid"

// LongTag decodes {"$long": "<decimal integer>"} to int64 (uint64 above the int64 range).
type LongTag struct{}

func (LongTag) Field() Field { return FieldLong }

func (LongTag) Decode(text string) (any, error) {
	if n, err := strconv.ParseInt(text, 10, 64); err == nil {
		return n, nil
	}
	return strconv.ParseUint(text, 10, 64)
}

// BytesTag decodes {"$bytes": "<base64>"} to []byte.
type BytesTag struct{}

func (BytesTag) Field() Field { return FieldBytes }

func (BytesTag) Decode(text string) (any, error) {
	return base64.StdEncoding.DecodeString(text)
}

// DecimalTag decodes {"$decimal": "<decimal>"} to *big.Rat.
type DecimalTag struct{}

func (DecimalTag) Field() Field { return FieldDecimal }

func (DecimalTag) Decode(text string) (any, error) {
	r, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, strconv.ErrSyntax
	}
	return r, nil
}

// InstantTag decodes {"$timestamp": "<RFC 3339>"} to a UTC time.Time.
type InstantTag struct{}

func (InstantTag) Field() Field { return FieldInstant }

func (InstantTag) Decode(text string) (any, error) {
	t, err := time.Parse(time.RFC3339Nano, text)
	return t.UTC(), err
}

// TagSet indexes tags by their member name.
type TagSet map[Field]Tag

// StandardTags are the tags of design §2.4.
var StandardTags = TagSet{
	FieldLong:    LongTag{},
	FieldBytes:   BytesTag{},
	FieldDecimal: DecimalTag{},
	FieldInstant: InstantTag{},
}

// TaggedDecoder turns guest JSON (decoded with UseNumber) back into Go values.
type TaggedDecoder struct {
	Tags TagSet
}

// Decode walks v: single-member tag objects become Go values, exact integers become int64.
func (d TaggedDecoder) Decode(v any) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		return d.object(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			decoded, err := d.Decode(item)
			if err != nil {
				return nil, err
			}
			out[i] = decoded
		}
		return out, nil
	case json.Number:
		return NumberValue{}.Of(t), nil
	}
	return v, nil
}

func (d TaggedDecoder) object(obj map[string]any) (any, error) {
	if tagged, ok, err := d.tag(obj); ok {
		return tagged, err
	}
	out := make(map[string]any, len(obj))
	for k, item := range obj {
		decoded, err := d.Decode(item)
		if err != nil {
			return nil, err
		}
		out[k] = decoded
	}
	return out, nil
}

// tag decodes obj when it is exactly one known tag member with a string value.
func (d TaggedDecoder) tag(obj map[string]any) (any, bool, error) {
	if len(obj) != 1 {
		return nil, false, nil
	}
	for name, raw := range obj {
		tag, known := d.Tags[Field(name)]
		text, isText := raw.(string)
		if !known || !isText {
			return nil, false, nil
		}
		v, err := tag.Decode(text)
		if err != nil {
			return nil, true, &OutputError{Reason: fmt.Sprintf(string(ReasonBadTag), name, text)}
		}
		return v, true, nil
	}
	return nil, false, nil
}

// NumberValue converts a JSON number from the guest: exact integers to int64, others to float64.
type NumberValue struct{}

func (NumberValue) Of(n json.Number) any {
	if i, err := n.Int64(); err == nil {
		return i
	}
	f, err := n.Float64()
	if err != nil {
		return n
	}
	return f
}
