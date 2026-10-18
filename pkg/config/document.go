package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrTrailingData reports content after the top-level JSON value.
var ErrTrailingData = errors.New("unexpected data after the JSON document")

// Document is one route document: raw bytes, a generic tree for schema checks and
// pointer lookups, and the typed RouteConfig decoded on first use.
type Document struct {
	raw       []byte
	tree      any
	once      sync.Once
	config    *RouteConfig
	configErr error
}

// Parse checks JSON syntax and builds the generic tree; numbers keep full precision.
func Parse(raw []byte) (*Document, error) {
	tree, err := JSONDecoder{}.Decode(raw)
	if err != nil {
		return nil, err
	}
	return &Document{raw: raw, tree: tree}, nil
}

// NewDocument serialises a typed config into a parsed Document.
func NewDocument(cfg *RouteConfig) (*Document, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

// Raw returns the original bytes.
func (d *Document) Raw() []byte { return d.raw }

// Tree returns the generic decoded document (maps, slices, json.Number).
func (d *Document) Tree() any { return d.tree }

// Config decodes the typed route config; call only after schema validation passed.
func (d *Document) Config() (*RouteConfig, error) {
	d.once.Do(func() {
		var cfg RouteConfig
		d.configErr = JSONDecoder{}.Into(d.raw, &cfg)
		d.config = &cfg
	})
	return d.config, d.configErr
}

// Lookup resolves a JSON Pointer against the generic tree.
func (d *Document) Lookup(p Pointer) (any, bool) {
	cur := d.tree
	for _, tok := range p.Tokens() {
		next, ok := TreeIndex{}.Child(cur, tok)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// JSONDecoder decodes JSON with json.Number and rejects trailing data.
type JSONDecoder struct{}

// Decode returns the generic tree of raw.
func (JSONDecoder) Decode(raw []byte) (any, error) {
	var v any
	if err := (JSONDecoder{}).Into(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// Into decodes raw into target.
func (JSONDecoder) Into(raw []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return ErrTrailingData
	}
	return nil
}

// TreeIndex navigates generic JSON trees by pointer token.
type TreeIndex struct{}

// Child returns the member or element addressed by tok.
func (TreeIndex) Child(node any, tok string) (any, bool) {
	switch n := node.(type) {
	case map[string]any:
		v, ok := n[tok]
		return v, ok
	case []any:
		var i int
		if _, err := fmt.Sscanf(tok, "%d", &i); err != nil || i < 0 || i >= len(n) {
			return nil, false
		}
		return n[i], true
	}
	return nil, false
}
