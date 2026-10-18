package enginetest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/codec/codectest"
)

// RegisteredSubject is one entry of a golden registry.json.
type RegisteredSubject struct {
	Subject string           `json:"subject"`
	Type    codec.SchemaType `json:"type"`
	Schema  json.RawMessage  `json:"schema"`
}

// WireAdapter is a fake schema registry plus JSON ↔ wire adapters for golden runs:
// input lines are encoded with the source format, outputs rendered back to JSON.
type WireAdapter struct {
	Registry *codectest.MemoryRegistry
}

// NewWireAdapter registers every subject of a registry.json document.
func NewWireAdapter(raw []byte) (*WireAdapter, error) {
	var subjects []RegisteredSubject
	if err := json.Unmarshal(raw, &subjects); err != nil {
		return nil, err
	}
	reg := codectest.NewMemoryRegistry()
	for _, s := range subjects {
		text := string(s.Schema)
		var str string
		if json.Unmarshal(s.Schema, &str) == nil {
			text = str
		}
		reg.Register(s.Subject, s.Type, text)
	}
	return &WireAdapter{Registry: reg}, nil
}

// Input returns the encoder of input lines for the route source format.
func (w *WireAdapter) Input(tb testing.TB, cfg *config.RouteConfig) *WireInput {
	tb.Helper()
	spec := cfg.Source.Format
	if spec.Subject == nil {
		s := cfg.Source.Topics[0] + "-value"
		spec.Subject = &s
	}
	c, err := codec.DefaultFactoryManager().NewSpec(codec.CodecSpec{Format: spec, Registry: w.Registry})
	if err != nil {
		tb.Fatal(err)
	}
	return &WireInput{codec: c}
}

// Decode renders produced bytes: framed values by their registry schema, others as JSON text.
func (w *WireAdapter) Decode(ctx context.Context, raw []byte) (any, error) {
	frame, err := codec.Framing{}.Parse(raw)
	if err != nil {
		return json.RawMessage(raw), nil
	}
	rs, err := w.Registry.SchemaByID(ctx, frame.SchemaID)
	if err != nil {
		return nil, err
	}
	format := wireFormats[rs.EffectiveType()]
	c, err := codec.DefaultFactoryManager().NewSpec(codec.CodecSpec{Format: config.Format{Type: format}, Registry: w.Registry})
	if err != nil {
		return nil, err
	}
	return c.Decode(ctx, raw)
}

// wireFormats maps registry schema types to route formats.
var wireFormats = map[codec.SchemaType]config.FormatType{codec.SchemaTypeAvro: config.FormatTypeAvro, codec.SchemaTypeProtobuf: config.FormatTypeProtobuf}

// WireInput encodes JSON input lines with a registry-backed codec.
type WireInput struct {
	codec codec.Codec
}

// Encode parses the JSON line and encodes it.
func (w *WireInput) Encode(ctx context.Context, line []byte) ([]byte, error) {
	v, err := codec.JSONCodec{}.Decode(ctx, line)
	if err != nil {
		return nil, err
	}
	return w.codec.Encode(ctx, v)
}
