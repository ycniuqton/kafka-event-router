package preview

import (
	"context"
	"encoding/base64"
	"unicode/utf8"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
)

// SampleDecoder decodes stored records with the session's value format (engine codecs); registry-backed
// formats (avro, protobuf) resolve the schema registry of the session's connection.
type SampleDecoder struct {
	Codecs     *codec.FactoryManager
	Registries codec.RegistryProvider
}

// NewSampleDecoder uses the engine's default codecs and the given registry provider (nil = none).
func NewSampleDecoder(registries codec.RegistryProvider) SampleDecoder {
	return SampleDecoder{Codecs: codec.DefaultFactoryManager(), Registries: registries}
}

// Supports reports whether format has a codec.
func (d SampleDecoder) Supports(format config.FormatType) bool {
	for _, f := range d.Codecs.Formats() {
		if f == format {
			return true
		}
	}
	return false
}

// SampleFormat is how a session's values are decoded: the format and the connection owning the registry.
type SampleFormat struct {
	Type         config.FormatType
	ConnectionID string
}

// SampleFormat returns the session's decode format.
func (s Session) SampleFormat() SampleFormat {
	return SampleFormat{Type: s.Format, ConnectionID: s.ConnectionID}
}

// Decoded is a record's decoded key/value; DecodeError is set when the value does not decode.
type Decoded struct {
	Key         any
	Value       any
	DecodeError *string
}

// For builds one codec for a batch of records (schemas are cached across the batch).
func (d SampleDecoder) For(f SampleFormat) RecordDecoder {
	c, err := d.Codecs.NewSpec(codec.CodecSpec{Format: config.Format{Type: f.Type}, Registry: d.registry(f)})
	return RecordDecoder{codec: c, err: err}
}

// registry is the lazily resolved registry of the connection (none for uploads or without a provider).
func (d SampleDecoder) registry(f SampleFormat) codec.SchemaRegistry {
	if d.Registries == nil || f.ConnectionID == "" {
		return codec.UnconfiguredRegistry{}
	}
	return codec.ConnectionRegistry{Provider: d.Registries, ConnectionID: f.ConnectionID}
}

// Decode decodes one record with format.
func (d SampleDecoder) Decode(ctx context.Context, rec SampleRecord, f SampleFormat) Decoded {
	return d.For(f).Decode(ctx, rec)
}

// RecordDecoder decodes records with one codec.
type RecordDecoder struct {
	codec codec.Codec
	err   error
}

// Decode decodes rec; a codec or payload failure becomes DecodeError.
func (r RecordDecoder) Decode(ctx context.Context, rec SampleRecord) Decoded {
	out := Decoded{Key: SampleDecoder{}.key(rec.Key)}
	err := r.err
	if err == nil {
		out.Value, err = r.codec.Decode(ctx, rec.Value)
	}
	if err != nil {
		msg := err.Error()
		out.DecodeError = &msg
		out.Value = nil
	}
	return out
}

// key renders the key as text when UTF-8, else base64.
func (SampleDecoder) key(b []byte) any {
	if b == nil {
		return nil
	}
	text, _ := BytesRenderer{}.Text(b)
	return text
}

// Document returns the event document schema inference walks ({key, value, headers}).
func (d Decoded) Document(rec SampleRecord) map[string]any {
	headers := map[string]any{}
	for _, h := range rec.Headers {
		headers[h.Key] = string(h.Value)
	}
	return map[string]any{string(engine.FieldKey): d.Key, string(engine.FieldValue): d.Value, string(engine.FieldHeaders): headers}
}

// SampleViewBuilder renders records as SampleDTOs.
type SampleViewBuilder struct {
	Decoder SampleDecoder
}

// DTOs renders every record with the session format.
func (b SampleViewBuilder) DTOs(ctx context.Context, recs []SampleRecord, format SampleFormat) []SampleDTO {
	dec := b.Decoder.For(format)
	out := make([]SampleDTO, 0, len(recs))
	for _, r := range recs {
		out = append(out, b.DTO(r, dec.Decode(ctx, r)))
	}
	return out
}

// DTO renders one decoded record, including the exact raw bytes as text.
func (b SampleViewBuilder) DTO(r SampleRecord, dec Decoded) SampleDTO {
	return SampleDTO{
		Seq: r.Seq, Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp,
		Key: dec.Key, Value: dec.Value, Headers: BytesRenderer{}.Headers(r.Headers), Raw: b.raw(r), DecodeError: dec.DecodeError,
	}
}

// raw renders key and value in one encoding: UTF-8 when both are valid text, else base64.
func (SampleViewBuilder) raw(r SampleRecord) RawDTO {
	enc := EncodingUTF8
	if !utf8.Valid(r.Value) || (r.Key != nil && !utf8.Valid(r.Key)) {
		enc = EncodingBase64
	}
	out := RawDTO{Value: RawText{}.Render(r.Value, enc), Encoding: enc}
	if r.Key != nil {
		key := RawText{}.Render(r.Key, enc)
		out.Key = &key
	}
	return out
}

// RawText renders bytes in a RawEncoding.
type RawText struct{}

// Render returns b as text or base64.
func (RawText) Render(b []byte, enc RawEncoding) string {
	if enc == EncodingBase64 {
		return base64.StdEncoding.EncodeToString(b)
	}
	return string(b)
}

// SchemaBuilder infers the schema of records and merges the registry schema's fields (avro, protobuf).
type SchemaBuilder struct {
	Decoder  SampleDecoder
	Inferrer SchemaInferrer
	Merger   RegistrySchemaMerger
}

// Infer decodes every record and infers the field schema.
func (b SchemaBuilder) Infer(ctx context.Context, recs []SampleRecord, format SampleFormat) FieldSchema {
	dec := b.Decoder.For(format)
	docs := make([]any, 0, len(recs))
	for _, r := range recs {
		docs = append(docs, dec.Decode(ctx, r).Document(r))
	}
	fs := b.Merger.Merge(ctx, recs, format, b.Inferrer.Infer(docs))
	fs.Format = format.Type
	return fs
}
