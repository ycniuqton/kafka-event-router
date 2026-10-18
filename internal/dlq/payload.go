package dlq

import (
	"context"
	"encoding/json"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
)

// SourceFormat is the source connection and key/value formats of one route version.
type SourceFormat struct {
	ConnectionID string
	Key          config.Format
	Value        config.Format
}

// SourceFormats maps version numbers to their source formats.
type SourceFormats map[int]SourceFormat

// Of returns the format of the version that wrote the dead letter.
func (f SourceFormats) Of(versionNo int) (SourceFormat, bool) {
	sf, ok := f[versionNo]
	return sf, ok
}

// SourceFormatsOf derives source formats from stored version configs.
type SourceFormatsOf struct{}

// Versions returns the formats of every parsable version (key defaults to string, like the engine).
func (SourceFormatsOf) Versions(versions []VersionConfig) SourceFormats {
	out := SourceFormats{}
	for _, v := range versions {
		cfg, ok := (StepLabelsOf{}).config(v)
		if !ok {
			continue
		}
		sf := SourceFormat{ConnectionID: cfg.Source.ConnectionID, Value: cfg.Source.Format, Key: config.Format{Type: config.FormatTypeString}}
		if cfg.Source.KeyFormat != nil {
			sf.Key = *cfg.Source.KeyFormat
		}
		out[v.No] = sf
	}
	return out
}

// SchemaEncodings maps registry-backed formats to the encoding shown in the detail (FR-10).
var SchemaEncodings = map[config.FormatType]Encoding{
	config.FormatTypeAvro:     EncodingAvro,
	config.FormatTypeProtobuf: EncodingProtobuf,
}

// PayloadDecoder renders key and value of a dead letter beyond the plain JSON/text/base64 view.
type PayloadDecoder interface {
	Decode(ctx context.Context, formats SourceFormats, rec record.Record, d DetailView) DetailView
}

// PlainPayloads keeps the JSON/text/base64 rendering (no schema registry).
type PlainPayloads struct{}

// Decode implements PayloadDecoder.
func (PlainPayloads) Decode(_ context.Context, _ SourceFormats, _ record.Record, d DetailView) DetailView {
	return d
}

// SchemaPayloads decodes Avro/Protobuf bytes through the engine codecs and the source connection's
// schema registry; undecodable bytes keep their plain rendering (base64 with the raw-bytes marker).
type SchemaPayloads struct {
	Codecs     *codec.FactoryManager
	Registries codec.RegistryProvider
}

// Decode implements PayloadDecoder.
func (p SchemaPayloads) Decode(ctx context.Context, formats SourceFormats, rec record.Record, d DetailView) DetailView {
	sf, ok := formats.Of(rec.Failure.VersionNo)
	if !ok {
		return d
	}
	d.Key = p.one(ctx, sf.ConnectionID, sf.Key, rec.Key, d.Key)
	d.Value = p.one(ctx, sf.ConnectionID, sf.Value, rec.Value, d.Value)
	return d
}

// one decodes raw with format when it is registry-backed; any failure keeps plain.
func (p SchemaPayloads) one(ctx context.Context, conn string, format config.Format, raw []byte, plain EncodedValue) EncodedValue {
	enc, schemaBacked := SchemaEncodings[format.Type]
	if !schemaBacked || raw == nil {
		return plain
	}
	c, err := p.Codecs.NewSpec(codec.CodecSpec{Format: format, Registry: codec.ConnectionRegistry{Provider: p.Registries, ConnectionID: conn}})
	if err != nil {
		return plain
	}
	v, err := c.Decode(ctx, raw)
	if err != nil {
		return plain
	}
	data, err := json.Marshal(v)
	if err != nil {
		return plain
	}
	out := EncodedValue{Encoding: enc, Data: json.RawMessage(data)}
	if frame, err := (codec.Framing{}).Parse(raw); err == nil {
		out.SchemaID = &frame.SchemaID
	}
	return out
}
