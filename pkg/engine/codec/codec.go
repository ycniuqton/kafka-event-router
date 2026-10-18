// Package codec decodes record bytes once at pipeline entry and encodes once at exit.
// Codecs are created per route by CodecFactory implementations selected by format.
package codec

import (
	"context"
	"errors"
	"fmt"

	"github.com/eventrouter/router/pkg/config"
)

// ErrUnsupportedFormat reports a format without a registered factory.
var ErrUnsupportedFormat = errors.New("unsupported format")

// ErrMalformed reports bytes that are not valid for the codec.
var ErrMalformed = errors.New("malformed payload")

// ErrUnencodable reports a value the codec cannot encode.
var ErrUnencodable = errors.New("value cannot be encoded")

// Codec converts between bytes and decoded values; nil bytes decode to nil.
type Codec interface {
	Format() config.FormatType
	Decode(ctx context.Context, raw []byte) (any, error)
	Encode(ctx context.Context, v any) ([]byte, error)
}

// CodecSpec is what a factory needs to build a codec: the format member of the document
// and, for registry-backed formats, the schema registry of the connection.
type CodecSpec struct {
	Format   config.Format
	Registry SchemaRegistry
}

// RegistryOrNone returns the spec registry or UnconfiguredRegistry.
func (s CodecSpec) RegistryOrNone() SchemaRegistry {
	if s.Registry == nil {
		return UnconfiguredRegistry{}
	}
	return s.Registry
}

// CodecFactory builds a codec for one spec.
type CodecFactory interface {
	Format() config.FormatType
	New(spec CodecSpec) (Codec, error)
}

// FactoryManager selects codec factories by format; business logic uses only this.
type FactoryManager struct {
	factories map[config.FormatType]CodecFactory
}

// NewFactoryManager registers the given factories.
func NewFactoryManager(fs ...CodecFactory) *FactoryManager {
	m := &FactoryManager{factories: map[config.FormatType]CodecFactory{}}
	for _, f := range fs {
		m.factories[f.Format()] = f
	}
	return m
}

// DefaultFactoryManager registers json, string, bytes, avro and protobuf.
func DefaultFactoryManager() *FactoryManager {
	return NewFactoryManager(JSONFactory{}, StringFactory{}, BytesFactory{}, AvroFactory{}, ProtobufFactory{})
}

// New creates the codec for a format without a schema registry.
func (m *FactoryManager) New(format config.Format) (Codec, error) {
	return m.NewSpec(CodecSpec{Format: format})
}

// NewSpec creates the codec for spec.
func (m *FactoryManager) NewSpec(spec CodecSpec) (Codec, error) {
	f, ok := m.factories[spec.Format.Type]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFormat, spec.Format.Type)
	}
	return f.New(spec)
}

// Formats lists registered formats.
func (m *FactoryManager) Formats() []config.FormatType {
	out := make([]config.FormatType, 0, len(m.factories))
	for f := range m.factories {
		out = append(out, f)
	}
	return out
}
