package codec

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"time"

	"github.com/hamba/avro/v2"
)

// ErrAvroValue is the cause when an engine value does not fit the Avro schema.
var ErrAvroValue = errors.New("value does not match the avro schema")

// AvroDateLayout renders the Avro date logical type.
const AvroDateLayout = time.DateOnly

// AvroValueMapper converts one Avro schema type between hamba generic values and engine
// values (int64/float64/string/bool/[]byte/maps/slices, dates and decimals as text).
type AvroValueMapper interface {
	Type() avro.Type
	ToEngine(m *AvroValues, s avro.Schema, v any) any
	ToAvro(m *AvroValues, s avro.Schema, v any) (any, error)
}

// AvroValues is the registry of type mappers selected by schema type.
type AvroValues struct {
	mappers  map[avro.Type]AvroValueMapper
	logicals map[avro.LogicalType]AvroLogicalMapper
}

// NewAvroValues registers the given mappers.
func NewAvroValues(ms []AvroValueMapper, ls []AvroLogicalMapper) *AvroValues {
	v := &AvroValues{mappers: map[avro.Type]AvroValueMapper{}, logicals: map[avro.LogicalType]AvroLogicalMapper{}}
	for _, m := range ms {
		v.mappers[m.Type()] = m
	}
	for _, l := range ls {
		v.logicals[l.Logical()] = l
	}
	return v
}

// DefaultAvroValues maps every Avro type and the common logical types.
func DefaultAvroValues() *AvroValues {
	return NewAvroValues(
		[]AvroValueMapper{
			avroRecord{typ: avro.Record}, avroRecord{typ: avro.Error}, avroRef{}, avroUnion{}, avroArray{}, avroMap{},
			avroEnum{}, avroFixed{}, avroNull{},
			avroScalar{typ: avro.String, to: AvroScalars{}.String}, avroScalar{typ: avro.Bytes, to: AvroScalars{}.Bytes},
			avroScalar{typ: avro.Int, to: AvroScalars{}.Int}, avroScalar{typ: avro.Long, to: AvroScalars{}.Long},
			avroScalar{typ: avro.Float, to: AvroScalars{}.Float}, avroScalar{typ: avro.Double, to: AvroScalars{}.Double},
			avroScalar{typ: avro.Boolean, to: AvroScalars{}.Boolean},
		},
		[]AvroLogicalMapper{
			avroTimestamp{logical: avro.TimestampMillis}, avroTimestamp{logical: avro.TimestampMicros},
			avroTimestamp{logical: avro.LocalTimestampMillis}, avroTimestamp{logical: avro.LocalTimestampMicros},
			avroDate{}, avroDecimal{}, avroTimeOfDay{logical: avro.TimeMillis, unit: time.Millisecond}, avroTimeOfDay{logical: avro.TimeMicros, unit: time.Microsecond},
		},
	)
}

// ToEngine converts a decoded value; unknown types pass through.
func (m *AvroValues) ToEngine(s avro.Schema, v any) any {
	if l, ok := m.logical(s); ok && v != nil {
		return l.ToEngine(s, v)
	}
	if mp, ok := m.mappers[s.Type()]; ok {
		return mp.ToEngine(m, s, v)
	}
	return v
}

// ToAvro converts an engine value for encoding with s.
func (m *AvroValues) ToAvro(s avro.Schema, v any) (any, error) {
	if l, ok := m.logical(s); ok {
		return l.ToAvro(s, v)
	}
	mp, ok := m.mappers[s.Type()]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported avro type %s", ErrAvroValue, s.Type())
	}
	return mp.ToAvro(m, s, v)
}

func (m *AvroValues) logical(s avro.Schema) (AvroLogicalMapper, bool) {
	ls, ok := s.(avro.LogicalTypeSchema)
	if !ok || ls.Logical() == nil {
		return nil, false
	}
	l, ok := m.logicals[ls.Logical().Type()]
	return l, ok
}

// AvroLogicalMapper converts one logical type.
type AvroLogicalMapper interface {
	Logical() avro.LogicalType
	ToEngine(s avro.Schema, v any) any
	ToAvro(s avro.Schema, v any) (any, error)
}

// avroRecord maps records to objects; absent fields use the schema default or null.
type avroRecord struct {
	typ avro.Type
}

func (r avroRecord) Type() avro.Type { return r.typ }

func (avroRecord) ToEngine(m *AvroValues, s avro.Schema, v any) any {
	in, ok := v.(map[string]any)
	if !ok {
		return v
	}
	out := make(map[string]any, len(in))
	for _, f := range s.(*avro.RecordSchema).Fields() {
		if fv, ok := in[f.Name()]; ok {
			out[f.Name()] = m.ToEngine(f.Type(), fv)
		}
	}
	return out
}

func (avroRecord) ToAvro(m *AvroValues, s avro.Schema, v any) (any, error) {
	in, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s needs an object, got %T", ErrAvroValue, s.(*avro.RecordSchema).FullName(), v)
	}
	out := make(map[string]any, len(in))
	for _, f := range s.(*avro.RecordSchema).Fields() {
		fv, present := in[f.Name()]
		if !present && f.HasDefault() {
			continue
		}
		av, err := m.ToAvro(f.Type(), fv)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", f.Name(), err)
		}
		out[f.Name()] = av
	}
	return out, nil
}

// avroRef follows named type references.
type avroRef struct{}

func (avroRef) Type() avro.Type { return avro.Ref }

func (avroRef) ToEngine(m *AvroValues, s avro.Schema, v any) any {
	return m.ToEngine(s.(*avro.RefSchema).Schema(), v)
}

func (avroRef) ToAvro(m *AvroValues, s avro.Schema, v any) (any, error) {
	return m.ToAvro(s.(*avro.RefSchema).Schema(), v)
}

// avroArray maps arrays item by item.
type avroArray struct{}

func (avroArray) Type() avro.Type { return avro.Array }

func (avroArray) ToEngine(m *AvroValues, s avro.Schema, v any) any {
	in, ok := v.([]any)
	if !ok {
		return v
	}
	out := make([]any, len(in))
	for i, e := range in {
		out[i] = m.ToEngine(s.(*avro.ArraySchema).Items(), e)
	}
	return out
}

func (avroArray) ToAvro(m *AvroValues, s avro.Schema, v any) (any, error) {
	in, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: array needs a list, got %T", ErrAvroValue, v)
	}
	out := make([]any, len(in))
	for i, e := range in {
		av, err := m.ToAvro(s.(*avro.ArraySchema).Items(), e)
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		out[i] = av
	}
	return out, nil
}

// avroMap maps string-keyed maps value by value.
type avroMap struct{}

func (avroMap) Type() avro.Type { return avro.Map }

func (avroMap) ToEngine(m *AvroValues, s avro.Schema, v any) any {
	in, ok := v.(map[string]any)
	if !ok {
		return v
	}
	out := make(map[string]any, len(in))
	for k, e := range in {
		out[k] = m.ToEngine(s.(*avro.MapSchema).Values(), e)
	}
	return out
}

func (avroMap) ToAvro(m *AvroValues, s avro.Schema, v any) (any, error) {
	in, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: map needs an object, got %T", ErrAvroValue, v)
	}
	out := make(map[string]any, len(in))
	for k, e := range in {
		av, err := m.ToAvro(s.(*avro.MapSchema).Values(), e)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		out[k] = av
	}
	return out, nil
}

// avroEnum maps enum symbols to strings.
type avroEnum struct{}

func (avroEnum) Type() avro.Type                                  { return avro.Enum }
func (avroEnum) ToEngine(_ *AvroValues, _ avro.Schema, v any) any { return v }

func (avroEnum) ToAvro(_ *AvroValues, s avro.Schema, v any) (any, error) {
	sym, ok := v.(string)
	for _, known := range s.(*avro.EnumSchema).Symbols() {
		if ok && known == sym {
			return sym, nil
		}
	}
	return nil, fmt.Errorf("%w: %v is not a symbol of %s", ErrAvroValue, v, s.(*avro.EnumSchema).FullName())
}

// avroFixed maps fixed-size byte arrays to byte slices.
type avroFixed struct{}

func (avroFixed) Type() avro.Type { return avro.Fixed }

func (avroFixed) ToEngine(_ *AvroValues, _ avro.Schema, v any) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Array {
		return v
	}
	out := make([]byte, rv.Len())
	reflect.Copy(reflect.ValueOf(out), rv)
	return out
}

func (avroFixed) ToAvro(_ *AvroValues, s avro.Schema, v any) (any, error) {
	b, err := AvroScalars{}.Bytes(v)
	size := s.(*avro.FixedSchema).Size()
	if err != nil || len(b.([]byte)) != size {
		return nil, fmt.Errorf("%w: fixed needs %d bytes", ErrAvroValue, size)
	}
	arr := reflect.New(reflect.ArrayOf(size, reflect.TypeFor[byte]())).Elem()
	reflect.Copy(arr, reflect.ValueOf(b))
	return arr.Interface(), nil
}

// avroNull accepts only null.
type avroNull struct{}

func (avroNull) Type() avro.Type                                  { return avro.Null }
func (avroNull) ToEngine(_ *AvroValues, _ avro.Schema, _ any) any { return nil }

func (avroNull) ToAvro(_ *AvroValues, _ avro.Schema, v any) (any, error) {
	if v != nil {
		return nil, fmt.Errorf("%w: null expected, got %T", ErrAvroValue, v)
	}
	return nil, nil
}

// avroScalar maps primitive types with a converter; decoded scalars are normalised.
type avroScalar struct {
	typ avro.Type
	to  func(v any) (any, error)
}

func (a avroScalar) Type() avro.Type { return a.typ }

func (avroScalar) ToEngine(_ *AvroValues, _ avro.Schema, v any) any {
	return AvroScalars{}.Normalize(v)
}

func (a avroScalar) ToAvro(_ *AvroValues, _ avro.Schema, v any) (any, error) { return a.to(v) }

// AvroScalars converts primitive values in both directions.
type AvroScalars struct{}

// Normalize turns hamba primitives into engine numbers.
func (AvroScalars) Normalize(v any) any {
	switch t := v.(type) {
	case int:
		return int64(t)
	case int32:
		return int64(t)
	case float32:
		return float64(t)
	}
	return v
}

// String accepts text only.
func (AvroScalars) String(v any) (any, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	return nil, fmt.Errorf("%w: string expected, got %T", ErrAvroValue, v)
}

// Bytes accepts byte slices and text.
func (AvroScalars) Bytes(v any) (any, error) {
	switch t := v.(type) {
	case []byte:
		return t, nil
	case string:
		return []byte(t), nil
	}
	return nil, fmt.Errorf("%w: bytes expected, got %T", ErrAvroValue, v)
}

// Int accepts whole numbers in int32 range.
func (s AvroScalars) Int(v any) (any, error) {
	i, err := s.whole(v)
	if err != nil || i < math.MinInt32 || i > math.MaxInt32 {
		return nil, fmt.Errorf("%w: int expected, got %v", ErrAvroValue, v)
	}
	return int(i), nil
}

// Long accepts whole numbers.
func (s AvroScalars) Long(v any) (any, error) { return s.whole(v) }

// Float accepts numbers.
func (s AvroScalars) Float(v any) (any, error) {
	f, err := s.number(v)
	return float32(f), err
}

// Double accepts numbers.
func (s AvroScalars) Double(v any) (any, error) { return s.number(v) }

// Boolean accepts booleans.
func (AvroScalars) Boolean(v any) (any, error) {
	if b, ok := v.(bool); ok {
		return b, nil
	}
	return nil, fmt.Errorf("%w: boolean expected, got %T", ErrAvroValue, v)
}

func (AvroScalars) number(v any) (float64, error) {
	switch t := v.(type) {
	case int64:
		return float64(t), nil
	case float64:
		return t, nil
	case int:
		return float64(t), nil
	}
	return 0, fmt.Errorf("%w: number expected, got %T", ErrAvroValue, v)
}

func (s AvroScalars) whole(v any) (int64, error) {
	if i, ok := v.(int64); ok {
		return i, nil
	}
	f, err := s.number(v)
	if err != nil || f != math.Trunc(f) || math.Abs(f) > math.MaxInt64 {
		return 0, fmt.Errorf("%w: whole number expected, got %v", ErrAvroValue, v)
	}
	return int64(f), nil
}

// avroTimestamp maps timestamp logical types to RFC 3339 text (epoch millis accepted on encode).
type avroTimestamp struct {
	logical avro.LogicalType
}

func (a avroTimestamp) Logical() avro.LogicalType { return a.logical }

func (avroTimestamp) ToEngine(_ avro.Schema, v any) any {
	if t, ok := v.(time.Time); ok {
		return AvroTimes{}.Text(t, time.RFC3339Nano)
	}
	return AvroScalars{}.Normalize(v)
}

func (avroTimestamp) ToAvro(_ avro.Schema, v any) (any, error) {
	return AvroTimes{}.Parse(v)
}

// avroDate maps the date logical type to YYYY-MM-DD text.
type avroDate struct{}

func (avroDate) Logical() avro.LogicalType { return avro.Date }

func (avroDate) ToEngine(_ avro.Schema, v any) any {
	if t, ok := v.(time.Time); ok {
		return AvroTimes{}.Text(t, AvroDateLayout)
	}
	return AvroScalars{}.Normalize(v)
}

func (avroDate) ToAvro(_ avro.Schema, v any) (any, error) {
	if s, ok := v.(string); ok {
		if t, err := time.Parse(AvroDateLayout, s); err == nil {
			return t, nil
		}
	}
	return AvroTimes{}.Parse(v)
}

// avroTimeOfDay maps time-millis/micros to a whole number of units since midnight.
type avroTimeOfDay struct {
	logical avro.LogicalType
	unit    time.Duration
}

func (a avroTimeOfDay) Logical() avro.LogicalType { return a.logical }

func (a avroTimeOfDay) ToEngine(_ avro.Schema, v any) any {
	if d, ok := v.(time.Duration); ok {
		return int64(d / a.unit)
	}
	return AvroScalars{}.Normalize(v)
}

func (a avroTimeOfDay) ToAvro(_ avro.Schema, v any) (any, error) {
	n, err := AvroScalars{}.whole(v)
	return time.Duration(n) * a.unit, err
}

// avroDecimal maps decimals to exact decimal text with the schema scale.
type avroDecimal struct{}

func (avroDecimal) Logical() avro.LogicalType { return avro.Decimal }

func (avroDecimal) ToEngine(s avro.Schema, v any) any {
	r, ok := v.(*big.Rat)
	if !ok {
		return v
	}
	return r.FloatString(AvroDecimals{}.Scale(s))
}

func (avroDecimal) ToAvro(_ avro.Schema, v any) (any, error) {
	r, ok := new(big.Rat).SetString(fmt.Sprint(v))
	if !ok || v == nil {
		return nil, fmt.Errorf("%w: decimal expected, got %v", ErrAvroValue, v)
	}
	return r, nil
}

// AvroDecimals reads decimal parameters.
type AvroDecimals struct{}

// Scale returns the decimal scale of s (0 when absent).
func (AvroDecimals) Scale(s avro.Schema) int {
	ls, ok := s.(avro.LogicalTypeSchema)
	if !ok {
		return 0
	}
	if d, ok := ls.Logical().(*avro.DecimalLogicalSchema); ok {
		return d.Scale()
	}
	return 0
}

// AvroTimes parses engine time values (RFC 3339 text or epoch milliseconds).
type AvroTimes struct{}

// RFC 3339 can only render four-digit years.
const (
	AvroMinTextYear = 0
	AvroMaxTextYear = 9999
)

// Text renders t in layout, or as epoch milliseconds when the year cannot be written as text.
func (AvroTimes) Text(t time.Time, layout string) any {
	if y := t.UTC().Year(); y < AvroMinTextYear || y > AvroMaxTextYear {
		return t.UnixMilli()
	}
	return t.UTC().Format(layout)
}

// Parse returns the time of v.
func (AvroTimes) Parse(v any) (time.Time, error) {
	switch t := v.(type) {
	case string:
		return time.Parse(time.RFC3339Nano, t)
	case int64:
		return time.UnixMilli(t).UTC(), nil
	case float64:
		return time.UnixMilli(int64(t)).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("%w: timestamp expected, got %T", ErrAvroValue, v)
}
