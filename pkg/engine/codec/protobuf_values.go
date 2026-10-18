package codec

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// ErrProtoValue is the cause when an engine value does not fit the protobuf field.
var ErrProtoValue = errors.New("value does not match the protobuf schema")

// ProtoWellKnown names well-known message types with a scalar engine form.
type ProtoWellKnown protoreflect.FullName

const (
	ProtoTimestamp ProtoWellKnown = "google.protobuf.Timestamp"
)

// Protobuf well-known field names.
const (
	ProtoFieldSeconds protoreflect.Name = "seconds"
	ProtoFieldNanos   protoreflect.Name = "nanos"
)

// ProtoScalar converts one field kind between protoreflect values and engine values.
type ProtoScalar interface {
	ToEngine(fd protoreflect.FieldDescriptor, v protoreflect.Value) any
	ToProto(fd protoreflect.FieldDescriptor, v any) (protoreflect.Value, error)
}

// ProtoMessageForm converts a whole message type (well-known types).
type ProtoMessageForm interface {
	Name() ProtoWellKnown
	ToEngine(m protoreflect.Message) any
	ToProto(m protoreflect.Message, v any) error
}

// ProtoValues maps dynamic messages to engine maps (proto field names, defaults included
// for scalars) and back; kinds and well-known types are registries.
type ProtoValues struct {
	scalars map[protoreflect.Kind]ProtoScalar
	forms   map[ProtoWellKnown]ProtoMessageForm
}

// DefaultProtoValues registers every scalar kind and the Timestamp form.
func DefaultProtoValues() *ProtoValues {
	ints := protoInt{}
	uints := protoUint{}
	scalars := map[protoreflect.Kind]ProtoScalar{
		protoreflect.BoolKind: protoBool{}, protoreflect.EnumKind: protoEnum{},
		protoreflect.Int32Kind: ints, protoreflect.Sint32Kind: ints, protoreflect.Sfixed32Kind: ints,
		protoreflect.Int64Kind: ints, protoreflect.Sint64Kind: ints, protoreflect.Sfixed64Kind: ints,
		protoreflect.Uint32Kind: uints, protoreflect.Fixed32Kind: uints, protoreflect.Uint64Kind: uints, protoreflect.Fixed64Kind: uints,
		protoreflect.FloatKind: protoFloat{}, protoreflect.DoubleKind: protoFloat{},
		protoreflect.StringKind: protoString{}, protoreflect.BytesKind: protoBytes{},
	}
	return &ProtoValues{scalars: scalars, forms: map[ProtoWellKnown]ProtoMessageForm{ProtoTimestamp: protoTimestamp{}}}
}

// ToEngine converts a message to its engine form.
func (p *ProtoValues) ToEngine(m protoreflect.Message) any {
	if form, ok := p.forms[ProtoWellKnown(m.Descriptor().FullName())]; ok {
		return form.ToEngine(m)
	}
	out := map[string]any{}
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if !m.Has(fd) && (fd.HasPresence() || fd.IsList() || fd.IsMap()) {
			continue
		}
		out[string(fd.Name())] = p.field(fd, m.Get(fd))
	}
	return out
}

func (p *ProtoValues) field(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	switch {
	case fd.IsList():
		l := v.List()
		out := make([]any, l.Len())
		for i := 0; i < l.Len(); i++ {
			out[i] = p.single(fd, l.Get(i))
		}
		return out
	case fd.IsMap():
		out := map[string]any{}
		v.Map().Range(func(k protoreflect.MapKey, mv protoreflect.Value) bool {
			out[k.String()] = p.single(fd.MapValue(), mv)
			return true
		})
		return out
	}
	return p.single(fd, v)
}

func (p *ProtoValues) single(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	if fd.Message() != nil {
		return p.ToEngine(v.Message())
	}
	return p.scalars[fd.Kind()].ToEngine(fd, v)
}

// ToProto fills m from an engine value; unknown members are ignored.
func (p *ProtoValues) ToProto(m protoreflect.Message, v any) error {
	if form, ok := p.forms[ProtoWellKnown(m.Descriptor().FullName())]; ok {
		return form.ToProto(m, v)
	}
	in, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: %s needs an object, got %T", ErrProtoValue, m.Descriptor().FullName(), v)
	}
	fields := m.Descriptor().Fields()
	for name, fv := range in {
		fd := fields.ByName(protoreflect.Name(name))
		if fd == nil {
			fd = fields.ByJSONName(name)
		}
		if fd == nil || fv == nil {
			continue
		}
		if err := p.setField(m, fd, fv); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func (p *ProtoValues) setField(m protoreflect.Message, fd protoreflect.FieldDescriptor, v any) error {
	switch {
	case fd.IsList():
		return p.setList(m, fd, v)
	case fd.IsMap():
		return p.setMap(m, fd, v)
	}
	pv, err := p.value(m, fd, v)
	if err != nil {
		return err
	}
	m.Set(fd, pv)
	return nil
}

func (p *ProtoValues) setList(m protoreflect.Message, fd protoreflect.FieldDescriptor, v any) error {
	items, ok := v.([]any)
	if !ok {
		return fmt.Errorf("%w: list expected, got %T", ErrProtoValue, v)
	}
	l := m.Mutable(fd).List()
	for i, item := range items {
		pv, err := p.listValue(l, fd, item)
		if err != nil {
			return fmt.Errorf("[%d]: %w", i, err)
		}
		l.Append(pv)
	}
	return nil
}

func (p *ProtoValues) listValue(l protoreflect.List, fd protoreflect.FieldDescriptor, v any) (protoreflect.Value, error) {
	if fd.Message() == nil {
		return p.scalars[fd.Kind()].ToProto(fd, v)
	}
	elem := l.NewElement()
	return elem, p.ToProto(elem.Message(), v)
}

func (p *ProtoValues) setMap(m protoreflect.Message, fd protoreflect.FieldDescriptor, v any) error {
	entries, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: object expected, got %T", ErrProtoValue, v)
	}
	mp := m.Mutable(fd).Map()
	for k, ev := range entries {
		key, err := p.scalars[fd.MapKey().Kind()].ToProto(fd.MapKey(), ProtoMapKeys{}.Parse(fd.MapKey(), k))
		if err != nil {
			return fmt.Errorf("key %s: %w", k, err)
		}
		val, err := p.mapValue(mp, fd.MapValue(), ev)
		if err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
		mp.Set(key.MapKey(), val)
	}
	return nil
}

func (p *ProtoValues) mapValue(mp protoreflect.Map, fd protoreflect.FieldDescriptor, v any) (protoreflect.Value, error) {
	if fd.Message() == nil {
		return p.scalars[fd.Kind()].ToProto(fd, v)
	}
	elem := mp.NewValue()
	return elem, p.ToProto(elem.Message(), v)
}

func (p *ProtoValues) value(m protoreflect.Message, fd protoreflect.FieldDescriptor, v any) (protoreflect.Value, error) {
	if fd.Message() == nil {
		return p.scalars[fd.Kind()].ToProto(fd, v)
	}
	child := m.NewField(fd)
	return child, p.ToProto(child.Message(), v)
}

// ProtoMapKeys parses textual map keys into the key kind.
type ProtoMapKeys struct{}

// Parse returns a typed key value for numeric and bool keys, the text otherwise.
func (ProtoMapKeys) Parse(fd protoreflect.FieldDescriptor, k string) any {
	if fd.Kind() == protoreflect.BoolKind {
		b, err := strconv.ParseBool(k)
		if err == nil {
			return b
		}
	}
	if i, err := strconv.ParseInt(k, 10, 64); err == nil && fd.Kind() != protoreflect.StringKind {
		return i
	}
	return k
}

// ProtoNumbers reads whole engine numbers.
type ProtoNumbers struct{}

// Whole returns v as int64 when it is a whole number.
func (ProtoNumbers) Whole(v any) (int64, error) {
	switch t := v.(type) {
	case int64:
		return t, nil
	case float64:
		if t == math.Trunc(t) && math.Abs(t) <= math.MaxInt64 {
			return int64(t), nil
		}
	case int:
		return int64(t), nil
	}
	return 0, fmt.Errorf("%w: whole number expected, got %v", ErrProtoValue, v)
}

type protoBool struct{}

func (protoBool) ToEngine(_ protoreflect.FieldDescriptor, v protoreflect.Value) any { return v.Bool() }
func (protoBool) ToProto(_ protoreflect.FieldDescriptor, v any) (protoreflect.Value, error) {
	b, ok := v.(bool)
	if !ok {
		return protoreflect.Value{}, fmt.Errorf("%w: boolean expected, got %T", ErrProtoValue, v)
	}
	return protoreflect.ValueOfBool(b), nil
}

type protoEnum struct{}

func (protoEnum) ToEngine(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	if ev := fd.Enum().Values().ByNumber(v.Enum()); ev != nil {
		return string(ev.Name())
	}
	return int64(v.Enum())
}

func (protoEnum) ToProto(fd protoreflect.FieldDescriptor, v any) (protoreflect.Value, error) {
	if s, ok := v.(string); ok {
		if ev := fd.Enum().Values().ByName(protoreflect.Name(s)); ev != nil {
			return protoreflect.ValueOfEnum(ev.Number()), nil
		}
	}
	n, err := ProtoNumbers{}.Whole(v)
	if err != nil || n < math.MinInt32 || n > math.MaxInt32 {
		return protoreflect.Value{}, fmt.Errorf("%w: %v is not a value of %s", ErrProtoValue, v, fd.Enum().FullName())
	}
	return protoreflect.ValueOfEnum(protoreflect.EnumNumber(n)), nil
}

// protoInt maps signed integer kinds (32-bit kinds range-checked).
type protoInt struct{}

func (protoInt) ToEngine(_ protoreflect.FieldDescriptor, v protoreflect.Value) any { return v.Int() }

func (protoInt) ToProto(fd protoreflect.FieldDescriptor, v any) (protoreflect.Value, error) {
	n, err := ProtoNumbers{}.Whole(v)
	if err != nil {
		return protoreflect.Value{}, err
	}
	if (ProtoBits{}).Is32(fd.Kind()) {
		if n < math.MinInt32 || n > math.MaxInt32 {
			return protoreflect.Value{}, fmt.Errorf("%w: %d overflows int32", ErrProtoValue, n)
		}
		return protoreflect.ValueOfInt32(int32(n)), nil
	}
	return protoreflect.ValueOfInt64(n), nil
}

// protoUint maps unsigned kinds; values above int64 become float64 in the engine.
type protoUint struct{}

func (protoUint) ToEngine(_ protoreflect.FieldDescriptor, v protoreflect.Value) any {
	u := v.Uint()
	if u > math.MaxInt64 {
		return float64(u)
	}
	return int64(u)
}

func (protoUint) ToProto(fd protoreflect.FieldDescriptor, v any) (protoreflect.Value, error) {
	n, err := ProtoNumbers{}.Whole(v)
	if err != nil || n < 0 {
		return protoreflect.Value{}, fmt.Errorf("%w: unsigned number expected, got %v", ErrProtoValue, v)
	}
	if (ProtoBits{}).Is32(fd.Kind()) {
		if n > math.MaxUint32 {
			return protoreflect.Value{}, fmt.Errorf("%w: %d overflows uint32", ErrProtoValue, n)
		}
		return protoreflect.ValueOfUint32(uint32(n)), nil
	}
	return protoreflect.ValueOfUint64(uint64(n)), nil
}

// ProtoBits tells 32-bit kinds from 64-bit kinds.
type ProtoBits struct{}

// bits32 are the 32-bit integer kinds.
var bits32 = map[protoreflect.Kind]bool{protoreflect.Int32Kind: true, protoreflect.Sint32Kind: true, protoreflect.Sfixed32Kind: true,
	protoreflect.Uint32Kind: true, protoreflect.Fixed32Kind: true}

// Is32 reports whether k is a 32-bit integer kind.
func (ProtoBits) Is32(k protoreflect.Kind) bool { return bits32[k] }

type protoFloat struct{}

func (protoFloat) ToEngine(_ protoreflect.FieldDescriptor, v protoreflect.Value) any {
	return v.Float()
}

func (protoFloat) ToProto(fd protoreflect.FieldDescriptor, v any) (protoreflect.Value, error) {
	var f float64
	switch t := v.(type) {
	case float64:
		f = t
	case int64:
		f = float64(t)
	default:
		return protoreflect.Value{}, fmt.Errorf("%w: number expected, got %T", ErrProtoValue, v)
	}
	if fd.Kind() == protoreflect.FloatKind {
		return protoreflect.ValueOfFloat32(float32(f)), nil
	}
	return protoreflect.ValueOfFloat64(f), nil
}

type protoString struct{}

func (protoString) ToEngine(_ protoreflect.FieldDescriptor, v protoreflect.Value) any {
	return v.String()
}

func (protoString) ToProto(_ protoreflect.FieldDescriptor, v any) (protoreflect.Value, error) {
	s, ok := v.(string)
	if !ok {
		return protoreflect.Value{}, fmt.Errorf("%w: string expected, got %T", ErrProtoValue, v)
	}
	return protoreflect.ValueOfString(s), nil
}

type protoBytes struct{}

func (protoBytes) ToEngine(_ protoreflect.FieldDescriptor, v protoreflect.Value) any {
	return append([]byte(nil), v.Bytes()...)
}

func (protoBytes) ToProto(_ protoreflect.FieldDescriptor, v any) (protoreflect.Value, error) {
	b, err := AvroScalars{}.Bytes(v)
	if err != nil {
		return protoreflect.Value{}, fmt.Errorf("%w: bytes expected, got %T", ErrProtoValue, v)
	}
	return protoreflect.ValueOfBytes(b.([]byte)), nil
}

// protoTimestamp maps google.protobuf.Timestamp to RFC 3339 text.
type protoTimestamp struct{}

func (protoTimestamp) Name() ProtoWellKnown { return ProtoTimestamp }

func (protoTimestamp) ToEngine(m protoreflect.Message) any {
	f := m.Descriptor().Fields()
	sec := m.Get(f.ByName(ProtoFieldSeconds)).Int()
	nanos := m.Get(f.ByName(ProtoFieldNanos)).Int()
	return time.Unix(sec, nanos).UTC().Format(time.RFC3339Nano)
}

func (protoTimestamp) ToProto(m protoreflect.Message, v any) error {
	t, err := AvroTimes{}.Parse(v)
	if err != nil {
		return fmt.Errorf("%w: timestamp expected, got %v", ErrProtoValue, v)
	}
	f := m.Descriptor().Fields()
	m.Set(f.ByName(ProtoFieldSeconds), protoreflect.ValueOfInt64(t.Unix()))
	m.Set(f.ByName(ProtoFieldNanos), protoreflect.ValueOfInt32(int32(t.Nanosecond())))
	return nil
}
