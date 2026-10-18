package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// DocFormatName names a route document serialisation.
type DocFormatName string

const (
	DocFormatJSON DocFormatName = "json"
	DocFormatYAML DocFormatName = "yaml"
)

// DocMediaType is the HTTP media type of a document format.
type DocMediaType string

const (
	MediaTypeJSON DocMediaType = "application/json"
	MediaTypeYAML DocMediaType = "application/yaml"
)

// YAMLTag is a core-schema YAML tag.
type YAMLTag string

const (
	TagStr   YAMLTag = "!!str"
	TagInt   YAMLTag = "!!int"
	TagFloat YAMLTag = "!!float"
	TagBool  YAMLTag = "!!bool"
	TagNull  YAMLTag = "!!null"
	TagMerge YAMLTag = "!!merge"
)

// YAMLNullText is the YAML spelling of null.
const YAMLNullText = "null"

// DocIndent is the indentation of rendered documents.
const DocIndent = 2

// MaxYAMLAliasDepth bounds alias resolution (billion-laughs style documents).
const MaxYAMLAliasDepth = 64

// ErrDocFormat reports an unknown format or content that cannot be converted losslessly.
var ErrDocFormat = errors.New("document format error")

// jsonNumber is the JSON number grammar; YAML numbers matching it are copied verbatim.
var jsonNumber = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// DocumentFormat converts between its serialisation and JSON document bytes, keeping
// member order and number text so a round trip is lossless.
type DocumentFormat interface {
	Name() DocFormatName
	MediaType() DocMediaType
	FromJSON(raw []byte) ([]byte, error)
	ToJSON(b []byte) ([]byte, error)
}

// DocumentFormatManager selects formats by name; routes import/export talks only to it.
type DocumentFormatManager struct {
	formats map[DocFormatName]DocumentFormat
}

// NewDocumentFormatManager registers the given formats.
func NewDocumentFormatManager(fs ...DocumentFormat) *DocumentFormatManager {
	m := &DocumentFormatManager{formats: map[DocFormatName]DocumentFormat{}}
	for _, f := range fs {
		m.formats[f.Name()] = f
	}
	return m
}

// DefaultDocumentFormats registers JSON and YAML.
func DefaultDocumentFormats() *DocumentFormatManager {
	return NewDocumentFormatManager(JSONDocFormat{}, YAMLDocFormat{})
}

// Get returns the format of name.
func (m *DocumentFormatManager) Get(name DocFormatName) (DocumentFormat, error) {
	f, ok := m.formats[name]
	if !ok {
		return nil, fmt.Errorf("%w: unknown format %q", ErrDocFormat, name)
	}
	return f, nil
}

// Convert re-serialises b from one format to another.
func (m *DocumentFormatManager) Convert(from, to DocFormatName, b []byte) ([]byte, error) {
	src, err := m.Get(from)
	if err != nil {
		return nil, err
	}
	dst, err := m.Get(to)
	if err != nil {
		return nil, err
	}
	raw, err := src.ToJSON(b)
	if err != nil {
		return nil, err
	}
	return dst.FromJSON(raw)
}

// JSONDocFormat renders indented JSON in document order.
type JSONDocFormat struct{}

func (JSONDocFormat) Name() DocFormatName     { return DocFormatJSON }
func (JSONDocFormat) MediaType() DocMediaType { return MediaTypeJSON }

func (JSONDocFormat) FromJSON(raw []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", string(bytes.Repeat([]byte(" "), DocIndent))); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDocFormat, err)
	}
	return append(out.Bytes(), '\n'), nil
}

func (JSONDocFormat) ToJSON(b []byte) ([]byte, error) {
	if _, err := (JSONDecoder{}).Decode(b); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDocFormat, err)
	}
	var out bytes.Buffer
	if err := json.Compact(&out, b); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDocFormat, err)
	}
	return out.Bytes(), nil
}

// YAMLDocFormat renders block-style YAML; strings that would read as other types are quoted.
type YAMLDocFormat struct{}

func (YAMLDocFormat) Name() DocFormatName     { return DocFormatYAML }
func (YAMLDocFormat) MediaType() DocMediaType { return MediaTypeYAML }

func (YAMLDocFormat) FromJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	node, err := (&jsonToYAML{dec: dec}).value()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDocFormat, err)
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(DocIndent)
	if err := enc.Encode(node); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDocFormat, err)
	}
	return out.Bytes(), enc.Close()
}

func (YAMLDocFormat) ToJSON(b []byte) ([]byte, error) {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	var doc, extra yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDocFormat, err)
	}
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, fmt.Errorf("%w: expected exactly one YAML document", ErrDocFormat)
	}
	var out bytes.Buffer
	if err := (yamlToJSON{out: &out}).write(doc.Content[0], 0); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDocFormat, err)
	}
	return out.Bytes(), nil
}

// jsonToYAML streams JSON tokens into a YAML node tree in member order.
type jsonToYAML struct {
	dec *json.Decoder
}

func (c *jsonToYAML) value() (*yaml.Node, error) {
	tok, err := c.dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		return c.container(t)
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: string(TagStr), Value: t}, nil
	case json.Number:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: c.numberTag(t), Value: t.String()}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: string(TagBool), Value: strconv.FormatBool(t)}, nil
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: string(TagNull), Value: YAMLNullText}, nil
}

func (c *jsonToYAML) numberTag(n json.Number) string {
	if _, err := strconv.ParseInt(n.String(), 10, 64); err == nil {
		return string(TagInt)
	}
	return string(TagFloat)
}

// container reads an object or array until its closing delimiter.
func (c *jsonToYAML) container(open json.Delim) (*yaml.Node, error) {
	n := &yaml.Node{Kind: yaml.SequenceNode}
	if open == '{' {
		n.Kind = yaml.MappingNode
	}
	for c.dec.More() {
		if n.Kind == yaml.MappingNode {
			key, err := c.value()
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, key)
		}
		v, err := c.value()
		if err != nil {
			return nil, err
		}
		n.Content = append(n.Content, v)
	}
	if _, err := c.dec.Token(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return n, nil
}

// yamlToJSON writes a YAML node tree as compact JSON in document order.
type yamlToJSON struct {
	out *bytes.Buffer
}

func (w yamlToJSON) write(n *yaml.Node, depth int) error {
	if depth > MaxYAMLAliasDepth {
		return errors.New("YAML nesting or aliases too deep")
	}
	switch n.Kind {
	case yaml.AliasNode:
		return w.write(n.Alias, depth+1)
	case yaml.MappingNode:
		return w.mapping(n, depth)
	case yaml.SequenceNode:
		return w.sequence(n, depth)
	case yaml.ScalarNode:
		return w.scalar(n)
	}
	return fmt.Errorf("unsupported YAML node at line %d", n.Line)
}

func (w yamlToJSON) mapping(n *yaml.Node, depth int) error {
	w.out.WriteByte('{')
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		if k.Kind != yaml.ScalarNode || k.Tag == string(TagMerge) {
			return fmt.Errorf("unsupported YAML key at line %d", k.Line)
		}
		if i > 0 {
			w.out.WriteByte(',')
		}
		if err := (JSONText{}).Write(w.out, k.Value); err != nil {
			return err
		}
		w.out.WriteByte(':')
		if err := w.write(n.Content[i+1], depth+1); err != nil {
			return err
		}
	}
	w.out.WriteByte('}')
	return nil
}

func (w yamlToJSON) sequence(n *yaml.Node, depth int) error {
	w.out.WriteByte('[')
	for i, item := range n.Content {
		if i > 0 {
			w.out.WriteByte(',')
		}
		if err := w.write(item, depth+1); err != nil {
			return err
		}
	}
	w.out.WriteByte(']')
	return nil
}

// scalar keeps JSON-compatible number text and converts other YAML spellings via decode.
func (w yamlToJSON) scalar(n *yaml.Node) error {
	tag := YAMLTag(n.ShortTag())
	if (tag == TagInt || tag == TagFloat) && jsonNumber.MatchString(n.Value) {
		w.out.WriteString(n.Value)
		return nil
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return err
	}
	if err := (JSONText{}).Write(w.out, v); err != nil {
		return fmt.Errorf("YAML value at line %d has no JSON form: %v", n.Line, err)
	}
	return nil
}

// JSONText writes JSON values without HTML escaping (documents keep `<`, `>` and `&`).
type JSONText struct{}

// Write appends the JSON encoding of v.
func (JSONText) Write(out *bytes.Buffer, v any) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	out.Write(bytes.TrimSuffix(b.Bytes(), []byte("\n")))
	return nil
}
