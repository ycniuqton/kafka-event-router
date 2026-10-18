// Package portability exports routes as YAML / JSON documents with connection names instead of ids and
// imports such documents back as drafts (never deploys). JSON↔YAML itself is pkg/config's DocumentFormat;
// this package only splits and joins multi-document payloads and resolves connection references.
package portability

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"

	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/pkg/config"
)

// DocumentCodec encodes route documents (JSON bytes each) into one payload and back.
type DocumentCodec interface {
	Format() routes.DocumentFormat
	MediaType() string
	Encode(docs []json.RawMessage) ([]byte, error)
	Decode(raw []byte) ([]json.RawMessage, error)
}

// YAMLSeparator starts every document of a multi-document YAML stream.
type YAMLSeparator string

const yamlDocStart YAMLSeparator = "---\n"

// DecodeError reports a payload that is not readable in its format (400 with the parser's line/column).
type DecodeError struct {
	Format routes.DocumentFormat
	Cause  error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("The %s could not be read: %v", e.Format, e.Cause)
}

func (e *DecodeError) Unwrap() error { return e.Cause }

// ProblemKind implements problem.Classified (400).
func (e *DecodeError) ProblemKind() problem.Kind { return problem.KindMalformedBody }

// YAMLCodec writes one "---"-separated document per route; reading also accepts JSON (YAML is a superset).
type YAMLCodec struct {
	Doc config.DocumentFormat
}

// NewYAMLCodec adapts pkg/config's YAML document format.
func NewYAMLCodec(formats *config.DocumentFormatManager) (YAMLCodec, error) {
	f, err := formats.Get(config.DocFormatYAML)
	return YAMLCodec{Doc: f}, err
}

func (YAMLCodec) Format() routes.DocumentFormat { return routes.FormatYAML }
func (c YAMLCodec) MediaType() string           { return string(c.Doc.MediaType()) }

// Encode renders each document and joins them with document separators.
func (c YAMLCodec) Encode(docs []json.RawMessage) ([]byte, error) {
	var out bytes.Buffer
	for _, d := range docs {
		y, err := c.Doc.FromJSON(d)
		if err != nil {
			return nil, err
		}
		out.WriteString(string(yamlDocStart))
		out.Write(y)
	}
	return out.Bytes(), nil
}

// Decode splits the stream into documents and converts each one to JSON; empty documents are skipped.
func (c YAMLCodec) Decode(raw []byte) ([]json.RawMessage, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	out := []json.RawMessage{}
	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, &DecodeError{Format: routes.FormatYAML, Cause: err}
		}
		doc, err := c.document(&node)
		if err != nil {
			return nil, &DecodeError{Format: routes.FormatYAML, Cause: err}
		}
		if doc != nil {
			out = append(out, doc)
		}
	}
}

// document re-serialises one parsed node and converts it with the lossless document format.
func (c YAMLCodec) document(node *yaml.Node) (json.RawMessage, error) {
	if c.empty(node) {
		return nil, nil
	}
	text, err := yaml.Marshal(node)
	if err != nil {
		return nil, err
	}
	return c.Doc.ToJSON(text)
}

// empty reports a document without content ("---" followed by nothing or an explicit null).
func (YAMLCodec) empty(node *yaml.Node) bool {
	if node.Kind == 0 || (node.Kind == yaml.DocumentNode && len(node.Content) == 0) {
		return true
	}
	if node.Kind != yaml.DocumentNode || len(node.Content) != 1 {
		return false
	}
	child := node.Content[0]
	return child.Kind == yaml.ScalarNode && child.Tag == string(config.TagNull)
}

// JSONCodec writes one object for a single route and an array for several; reading accepts both.
type JSONCodec struct {
	Doc config.DocumentFormat
}

// NewJSONCodec adapts pkg/config's JSON document format.
func NewJSONCodec(formats *config.DocumentFormatManager) (JSONCodec, error) {
	f, err := formats.Get(config.DocFormatJSON)
	return JSONCodec{Doc: f}, err
}

func (JSONCodec) Format() routes.DocumentFormat { return routes.FormatJSON }
func (c JSONCodec) MediaType() string           { return string(c.Doc.MediaType()) }

// Encode renders an indented object (one document) or array (several).
func (c JSONCodec) Encode(docs []json.RawMessage) ([]byte, error) {
	if len(docs) == 1 {
		return c.Doc.FromJSON(docs[0])
	}
	arr, err := json.Marshal(docs)
	if err != nil {
		return nil, err
	}
	return c.Doc.FromJSON(arr)
}

// Decode accepts one document object or an array of them.
func (c JSONCodec) Decode(raw []byte) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == byte(jsonArrayStart) {
		var docs []json.RawMessage
		if err := json.Unmarshal(trimmed, &docs); err != nil {
			return nil, &DecodeError{Format: routes.FormatJSON, Cause: err}
		}
		return docs, nil
	}
	if _, err := c.Doc.ToJSON(trimmed); err != nil {
		return nil, &DecodeError{Format: routes.FormatJSON, Cause: err}
	}
	return []json.RawMessage{json.RawMessage(trimmed)}, nil
}

// JSONMarker names structural bytes of JSON text.
type JSONMarker byte

const jsonArrayStart JSONMarker = '['

// CodecManager selects the codec of a format (the only entry point handlers use).
type CodecManager struct {
	codecs map[routes.DocumentFormat]DocumentCodec
}

// NewCodecManager registers codecs.
func NewCodecManager(cs ...DocumentCodec) *CodecManager {
	m := &CodecManager{codecs: map[routes.DocumentFormat]DocumentCodec{}}
	for _, c := range cs {
		m.codecs[c.Format()] = c
	}
	return m
}

// DefaultCodecs registers YAML and JSON over pkg/config's default document formats.
func DefaultCodecs() (*CodecManager, error) {
	formats := config.DefaultDocumentFormats()
	y, err := NewYAMLCodec(formats)
	if err != nil {
		return nil, err
	}
	j, err := NewJSONCodec(formats)
	if err != nil {
		return nil, err
	}
	return NewCodecManager(y, j), nil
}

// Codec returns the codec of format; "" means YAML.
func (m *CodecManager) Codec(format routes.DocumentFormat) (DocumentCodec, error) {
	if format == "" {
		format = routes.FormatYAML
	}
	c, ok := m.codecs[format]
	if !ok {
		return nil, ErrUnknownFormat
	}
	return c, nil
}
