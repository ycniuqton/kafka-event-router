package codec

import (
	"fmt"
	"math/big"
	"reflect"
	"time"

	"github.com/hamba/avro/v2"
)

// unionWrapped are member types hamba represents as {"<name>": value} inside unions.
var unionWrapped = map[avro.Type]bool{avro.Record: true, avro.Error: true, avro.Enum: true, avro.Fixed: true,
	avro.Array: true, avro.Map: true, avro.Ref: true}

// unionMatchers decide which primitive member produced a decoded (unwrapped) value.
var unionMatchers = map[avro.Type]func(v any) bool{
	avro.String:  func(v any) bool { _, ok := v.(string); return ok },
	avro.Boolean: func(v any) bool { _, ok := v.(bool); return ok },
	avro.Float:   func(v any) bool { _, ok := v.(float32); return ok },
	avro.Double:  func(v any) bool { _, ok := v.(float64); return ok },
	avro.Bytes: func(v any) bool {
		_, b := v.([]byte)
		_, r := v.(*big.Rat)
		return b || r
	},
	avro.Int:  func(v any) bool { return UnionKinds{}.IntLike(v, reflect.Int, reflect.Int32) },
	avro.Long: func(v any) bool { return UnionKinds{}.IntLike(v, reflect.Int64) },
}

// UnionKinds classifies decoded union values.
type UnionKinds struct{}

// IntLike reports integers of the given kinds and the time values hamba decodes from them.
func (UnionKinds) IntLike(v any, kinds ...reflect.Kind) bool {
	if _, ok := v.(time.Time); ok {
		return true
	}
	if _, ok := v.(time.Duration); ok {
		return true
	}
	k := reflect.ValueOf(v).Kind()
	for _, want := range kinds {
		if k == want {
			return true
		}
	}
	return false
}

// avroUnion unwraps named/complex members on decode and picks the first fitting member on encode.
type avroUnion struct{}

func (avroUnion) Type() avro.Type { return avro.Union }

func (u avroUnion) ToEngine(m *AvroValues, s avro.Schema, v any) any {
	if v == nil {
		return nil
	}
	members := s.(*avro.UnionSchema).Types()
	if w, ok := v.(map[string]any); ok && len(w) == 1 {
		for _, member := range members {
			if inner, hit := w[u.name(member)]; hit && unionWrapped[member.Type()] {
				return m.ToEngine(member, inner)
			}
		}
	}
	for _, member := range members {
		if match := unionMatchers[member.Type()]; match != nil && match(v) {
			return m.ToEngine(member, v)
		}
	}
	return AvroScalars{}.Normalize(v)
}

func (u avroUnion) ToAvro(m *AvroValues, s avro.Schema, v any) (any, error) {
	us := s.(*avro.UnionSchema)
	if v == nil {
		if us.Contains(avro.Null) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: null is not allowed by %s", ErrAvroValue, us.String())
	}
	for _, member := range us.Types() {
		if member.Type() == avro.Null {
			continue
		}
		av, err := m.ToAvro(member, v)
		if err != nil {
			continue
		}
		if unionWrapped[member.Type()] {
			return map[string]any{u.name(member): av}, nil
		}
		return av, nil
	}
	return nil, fmt.Errorf("%w: %T fits no member of %s", ErrAvroValue, v, us.String())
}

// name returns the union branch name hamba uses for a member.
func (avroUnion) name(s avro.Schema) string {
	if ref, ok := s.(*avro.RefSchema); ok {
		return ref.Schema().FullName()
	}
	if n, ok := s.(avro.NamedSchema); ok {
		return n.FullName()
	}
	return string(s.Type())
}
