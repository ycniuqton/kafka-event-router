package dlq

import (
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
)

// ErrInvalidRemoteOutput is the cause of a remote output that failed destination validation.
var ErrInvalidRemoteOutput = errors.New("output rejected by the destination format")

// Rendering tells how preview rendered an encoded output member back to bytes.
type Rendering string

const (
	RenderJSON   Rendering = "json"
	RenderText   Rendering = "text"
	RenderBase64 Rendering = "base64"
)

// RenderingOf maps destination format types to how preview renders their bytes (JSON as JSON, text as
// string, binary formats as base64).
var RenderingOf = map[config.FormatType]Rendering{
	config.FormatTypeJSON: RenderJSON, config.FormatTypeString: RenderText, config.FormatTypeBytes: RenderBase64,
	config.FormatTypeAvro: RenderBase64, config.FormatTypeProtobuf: RenderBase64,
}

// BranchRendering is the key and value rendering of one branch destination.
type BranchRendering struct {
	Key   Rendering
	Value Rendering
}

// RemoteOutputs turns rendered remote outputs back into destination records per branch.
type RemoteOutputs struct {
	Connections map[string]string
	Renderings  map[string]BranchRendering
	JSONSource  bool
}

// ErrInlineDecode is the cause of a JSON-source record whose bytes are not JSON (inline events would wrap them as text).
var ErrInlineDecode = errors.New("value is not valid JSON for the route's json source format")

// Undecodable reports records the preview service cannot receive faithfully as inline events.
func (o RemoteOutputs) Undecodable(rec record.Record) bool {
	return o.JSONSource && rec.Value != nil && !json.Valid(rec.Value)
}

// RemoteOutputsOf derives destinations and renderings from a version config.
type RemoteOutputsOf struct{}

// Version returns the remote outputs of v (same_as_source uses the source format).
func (RemoteOutputsOf) Version(v VersionConfig) (RemoteOutputs, error) {
	doc, err := config.Parse(v.Config)
	if err != nil {
		return RemoteOutputs{}, err
	}
	cfg, err := doc.Config()
	if err != nil {
		return RemoteOutputs{}, err
	}
	out := RemoteOutputs{Connections: map[string]string{}, Renderings: map[string]BranchRendering{}, JSONSource: cfg.Source.Format.Type == config.FormatTypeJSON}
	key := (RemoteOutputsOf{}).render(config.FormatTypeString)
	if cfg.Source.KeyFormat != nil {
		key = (RemoteOutputsOf{}).render(cfg.Source.KeyFormat.Type)
	}
	for _, b := range cfg.Branches {
		value := cfg.Source.Format.Type
		if f := b.Destination.Format; f != nil && f.Type != config.DestinationFormatTypeSameAsSource {
			value = config.FormatType(f.Type)
		}
		out.Connections[b.ID] = b.Destination.ConnectionID
		out.Renderings[b.ID] = BranchRendering{Key: key, Value: (RemoteOutputsOf{}).render(value)}
	}
	return out, nil
}

func (RemoteOutputsOf) render(t config.FormatType) Rendering {
	if r, ok := RenderingOf[t]; ok {
		return r
	}
	return RenderText
}

// Raw converts one rendered output into a destination record.
func (o RemoteOutputs) Raw(branch string, dto preview.OutputDTO) (engine.RawRecord, error) {
	if !dto.Valid {
		return engine.RawRecord{}, ErrInvalidRemoteOutput
	}
	r := o.Renderings[branch]
	key, err := (RenderedBytes{}).Of(r.Key, dto.Key)
	if err != nil {
		return engine.RawRecord{}, err
	}
	value, err := (RenderedBytes{}).Of(r.Value, dto.Value)
	if err != nil {
		return engine.RawRecord{}, err
	}
	hs := make([]engine.Header, 0, len(dto.Headers))
	for _, h := range dto.Headers {
		hs = append(hs, engine.Header{Key: h.Key, Value: []byte(h.Value)})
	}
	return engine.RawRecord{Topic: dto.Topic, Key: key, Value: value, Headers: hs}, nil
}

// RenderedBytes decodes rendered members.
type RenderedBytes struct{}

// Of returns the bytes of v rendered as r (nil stays nil).
func (RenderedBytes) Of(r Rendering, v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	text, isText := v.(string)
	switch {
	case r == RenderBase64 && isText:
		return base64.StdEncoding.DecodeString(text)
	case isText && r != RenderJSON:
		return []byte(text), nil
	}
	return json.Marshal(v)
}
