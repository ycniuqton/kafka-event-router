package codec

import (
	"context"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/eventrouter/router/pkg/config"
)

// StringFactory builds StringCodecs.
type StringFactory struct{}

func (StringFactory) Format() config.FormatType { return config.FormatTypeString }

func (StringFactory) New(CodecSpec) (Codec, error) { return StringCodec{}, nil }

// StringCodec decodes UTF-8 text; it encodes strings, bytes, numbers and booleans.
type StringCodec struct{}

func (StringCodec) Format() config.FormatType { return config.FormatTypeString }

func (StringCodec) Decode(_ context.Context, raw []byte) (any, error) {
	if raw == nil {
		return nil, nil
	}
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("%w: invalid UTF-8", ErrMalformed)
	}
	return string(raw), nil
}

func (StringCodec) Encode(_ context.Context, v any) ([]byte, error) {
	s, ok := ScalarText{}.Of(v)
	if !ok {
		return nil, fmt.Errorf("%w: %T as string", ErrUnencodable, v)
	}
	return s, nil
}

// BytesFactory builds BytesCodecs.
type BytesFactory struct{}

func (BytesFactory) Format() config.FormatType { return config.FormatTypeBytes }

func (BytesFactory) New(CodecSpec) (Codec, error) { return BytesCodec{}, nil }

// BytesCodec passes bytes through unchanged.
type BytesCodec struct{}

func (BytesCodec) Format() config.FormatType { return config.FormatTypeBytes }

func (BytesCodec) Decode(_ context.Context, raw []byte) (any, error) {
	if raw == nil {
		return nil, nil
	}
	return append([]byte(nil), raw...), nil
}

func (BytesCodec) Encode(_ context.Context, v any) ([]byte, error) {
	s, ok := ScalarText{}.Of(v)
	if !ok {
		return nil, fmt.Errorf("%w: %T as bytes", ErrUnencodable, v)
	}
	return s, nil
}

// ScalarText renders scalar values as bytes; containers are not scalar.
type ScalarText struct{}

// Of returns the bytes of v and whether v is a supported scalar (nil gives nil bytes).
func (ScalarText) Of(v any) ([]byte, bool) {
	switch t := v.(type) {
	case nil:
		return nil, true
	case string:
		return []byte(t), true
	case []byte:
		return t, true
	case bool:
		return []byte(strconv.FormatBool(t)), true
	case int64:
		return []byte(strconv.FormatInt(t, 10)), true
	case float64:
		return []byte(strconv.FormatFloat(t, 'f', -1, 64)), true
	case int:
		return []byte(strconv.Itoa(t)), true
	}
	return nil, false
}
