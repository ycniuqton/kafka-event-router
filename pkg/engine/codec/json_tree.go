package codec

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"slices"
)

// jsonKind names the token kinds the tree reader distinguishes.
type jsonKind = jsontext.Kind

const (
	kindNull   jsonKind = 'n'
	kindFalse  jsonKind = 'f'
	kindTrue   jsonKind = 't'
	kindString jsonKind = '"'
	kindNumber jsonKind = '0'
	kindObject jsonKind = '{'
	kindArray  jsonKind = '['
	kindEndObj jsonKind = '}'
	kindEndArr jsonKind = ']'
)

// readerOptions mirror encoding/json v1 leniency: duplicate names (last wins) and invalid UTF-8 allowed.
var readerOptions = []jsontext.Options{jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true)}

// TreeReader decodes JSON into map[string]any / []any without reflection.
type TreeReader struct{}

// Read decodes exactly one value; integers become int64, other numbers float64.
func (TreeReader) Read(raw []byte) (any, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(raw), readerOptions...)
	v, err := treeBuilder{dec: dec}.value()
	if err != nil {
		return nil, err
	}
	if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data")
	}
	return v, nil
}

// treeBuilder consumes tokens recursively.
type treeBuilder struct {
	dec *jsontext.Decoder
}

func (b treeBuilder) value() (any, error) {
	tok, err := b.dec.ReadToken()
	if err != nil {
		return nil, err
	}
	switch tok.Kind() {
	case kindNull:
		return nil, nil
	case kindFalse, kindTrue:
		return tok.Bool(), nil
	case kindString:
		return tok.String(), nil
	case kindNumber:
		return b.number(tok.String()), nil
	case kindObject:
		return b.object()
	case kindArray:
		return b.array()
	}
	return nil, fmt.Errorf("unexpected token %v", tok.Kind())
}

func (treeBuilder) number(s string) any {
	return NumberPolicy{}.number(json.Number(s))
}

func (b treeBuilder) object() (any, error) {
	m := map[string]any{}
	for b.dec.PeekKind() != kindEndObj {
		tok, err := b.dec.ReadToken()
		if err != nil {
			return nil, err
		}
		name := tok.String()
		v, err := b.value()
		if err != nil {
			return nil, err
		}
		m[name] = v
	}
	_, err := b.dec.ReadToken()
	return m, err
}

func (b treeBuilder) array() (any, error) {
	arr := []any{}
	for b.dec.PeekKind() != kindEndArr {
		v, err := b.value()
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
	}
	_, err := b.dec.ReadToken()
	return arr, err
}

// TreeWriter encodes generic trees with sorted object keys and no HTML escaping.
type TreeWriter struct{}

// Write encodes v; types outside the generic tree fall back to encoding/json.
func (TreeWriter) Write(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := jsontext.NewEncoder(&buf, jsontext.AllowInvalidUTF8(true), jsontext.EscapeForHTML(false))
	if err := (treeEmitter{enc: enc}).value(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// treeEmitter writes tokens recursively.
type treeEmitter struct {
	enc *jsontext.Encoder
}

func (e treeEmitter) value(v any) error {
	switch t := v.(type) {
	case nil:
		return e.enc.WriteToken(jsontext.Null)
	case bool:
		return e.enc.WriteToken(jsontext.Bool(t))
	case string:
		return e.enc.WriteToken(jsontext.String(t))
	case int64:
		return e.enc.WriteToken(jsontext.Int(t))
	case float64:
		return e.enc.WriteToken(jsontext.Float(t))
	case map[string]any:
		return e.object(t)
	case []any:
		return e.array(t)
	}
	return e.fallback(v)
}

func (e treeEmitter) object(m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if err := e.enc.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	for _, k := range keys {
		if err := e.enc.WriteToken(jsontext.String(k)); err != nil {
			return err
		}
		if err := e.value(m[k]); err != nil {
			return err
		}
	}
	return e.enc.WriteToken(jsontext.EndObject)
}

func (e treeEmitter) array(a []any) error {
	if err := e.enc.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}
	for _, v := range a {
		if err := e.value(v); err != nil {
			return err
		}
	}
	return e.enc.WriteToken(jsontext.EndArray)
}

// fallback encodes non-tree values (json.Number, []byte, structs) with encoding/json.
func (e treeEmitter) fallback(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return e.enc.WriteValue(b)
}
