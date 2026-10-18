package boundary

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"time"
)

// MarshallerTagged is the boundary of Avro/Protobuf routes: lossless tagged values.
const MarshallerTagged MarshallerKind = "tagged"

// Tag members of tagged values (design §2.4); "$bytes" is FieldBytes.
const (
	FieldLong    Field = "$long"
	FieldDecimal Field = "$decimal"
	FieldInstant Field = "$timestamp"
)

// SafeInteger bounds the integers JavaScript numbers hold exactly (±(2^53 - 1)).
type SafeInteger int64

const MaxSafeInteger SafeInteger = 1<<53 - 1

// Contains reports whether n round-trips through a JS number.
func (s SafeInteger) Contains(n int64) bool {
	return n >= -int64(s) && n <= int64(s)
}

// TaggedMarshaller renders schema-typed values losslessly: int64 beyond 2^53 as {"$long"},
// bytes as {"$bytes"}, decimals as {"$decimal"} and timestamps as {"$timestamp"}.
type TaggedMarshaller struct{}

func (TaggedMarshaller) Kind() MarshallerKind { return MarshallerTagged }

// ToGuest renders the guest event with tagged key and value members.
func (TaggedMarshaller) ToGuest(ev GuestEvent) ([]byte, error) {
	return eventWriter{values: taggedValues{}}.write(ev)
}

// FromGuest validates the result and turns tags (and exact integers) back into Go values.
func (TaggedMarshaller) FromGuest(raw []byte) (Decoded, error) {
	return resultReader{values: taggedValues{}}.read(raw)
}

// taggedValues is the ValueCodec of TaggedMarshaller.
type taggedValues struct{}

func (taggedValues) ToGuest(v any) any            { return TaggedEncoder{}.Encode(v) }
func (taggedValues) FromGuest(v any) (any, error) { return TaggedDecoder{Tags: StandardTags}.Decode(v) }
func (taggedValues) UseNumber() bool              { return true }

// TaggedEncoder walks a Go value and replaces lossy scalars with tagged objects.
type TaggedEncoder struct{}

// Encode returns v with maps and slices walked and lossy scalars tagged.
func (e TaggedEncoder) Encode(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = e.Encode(item)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = e.Encode(item)
		}
		return out
	}
	return e.scalar(v)
}

func (e TaggedEncoder) scalar(v any) any {
	switch t := v.(type) {
	case []byte:
		return map[Field]string{FieldBytes: base64.StdEncoding.EncodeToString(t)}
	case time.Time:
		return map[Field]string{FieldInstant: t.UTC().Format(time.RFC3339Nano)}
	case *big.Rat:
		return map[Field]string{FieldDecimal: DecimalText{}.Format(t)}
	case big.Rat:
		return map[Field]string{FieldDecimal: DecimalText{}.Format(&t)}
	case *big.Int:
		return e.bigInt(t)
	}
	return e.integer(v)
}

func (TaggedEncoder) bigInt(n *big.Int) any {
	if n.IsInt64() && MaxSafeInteger.Contains(n.Int64()) {
		return n.Int64()
	}
	return map[Field]string{FieldLong: n.String()}
}

func (e TaggedEncoder) integer(v any) any {
	switch t := v.(type) {
	case int64:
		return e.long(t, v)
	case int:
		return e.long(int64(t), v)
	case uint64:
		if t > math.MaxInt64 {
			return map[Field]string{FieldLong: strconv.FormatUint(t, 10)}
		}
		return e.long(int64(t), v)
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return e.long(n, v)
		}
	}
	return v
}

func (TaggedEncoder) long(n int64, original any) any {
	if MaxSafeInteger.Contains(n) {
		return original
	}
	return map[Field]string{FieldLong: strconv.FormatInt(n, 10)}
}

// DecimalPrecision bounds rendering of decimals that have no finite decimal form.
type DecimalPrecision int

const DecimalFallbackScale DecimalPrecision = 38

// DecimalText renders a rational as an exact decimal string (Avro decimals always terminate).
type DecimalText struct{}

// Format returns the shortest exact decimal string of r.
func (DecimalText) Format(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	scale, ok := DecimalText{}.scale(r.Denom())
	if !ok {
		return r.FloatString(int(DecimalFallbackScale))
	}
	return r.FloatString(scale)
}

// scale returns the digits needed after the point, or false when 1/denom does not terminate.
func (DecimalText) scale(denom *big.Int) (int, bool) {
	d := new(big.Int).Set(denom)
	twos, fives := 0, 0
	two, five, rem := big.NewInt(2), big.NewInt(5), new(big.Int)
	for rem.Mod(d, two).Sign() == 0 {
		d.Quo(d, two)
		twos++
	}
	for rem.Mod(d, five).Sign() == 0 {
		d.Quo(d, five)
		fives++
	}
	return max(twos, fives), d.IsInt64() && d.Int64() == 1
}
