package infer

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/eventrouter/router/pkg/engine/path"
)

// Tree is a decoded JSON object with its addressable leaves in document-stable (sorted) order.
type Tree struct {
	Root   map[string]any
	Leaves []Leaf
}

// LeafFlattener decodes an object and lists its leaves; keys the path grammar cannot address are skipped.
type LeafFlattener struct {
	Limits Limits
}

// Flatten returns the tree of raw (must be a JSON object).
func (f LeafFlattener) Flatten(raw json.RawMessage) (Tree, bool) {
	var root map[string]any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil || root == nil {
		return Tree{}, false
	}
	t := Tree{Root: root}
	f.walk(root, nil, &t)
	return t, true
}

// walk appends the leaves under obj (prefix = its segments).
func (f LeafFlattener) walk(obj map[string]any, prefix []string, t *Tree) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		segs := append(append([]string{}, prefix...), k)
		if !(PathRules{}).Addressable(segs) || len(t.Leaves) >= f.Limits.MaxLeaves {
			continue
		}
		child, isObj := obj[k].(map[string]any)
		if isObj && len(child) > 0 {
			f.walk(child, segs, t)
			continue
		}
		t.Leaves = append(t.Leaves, Leaf{Segments: segs, Value: obj[k], Kind: Values{}.Kind(obj[k])})
	}
}

// PathRules checks segments against the engine's field path grammar.
type PathRules struct{}

// PathChars names character sets of the path grammar.
type PathChars string

// ReservedChars cannot appear in a member name (the grammar has no quoting).
const ReservedChars PathChars = PathChars(path.SyntaxDot + path.SyntaxIndexOpen + path.SyntaxIndexClose)

// Addressable reports whether every segment is a plain member name and the whole path parses.
func (PathRules) Addressable(segs []string) bool {
	for _, s := range segs {
		if s == "" || strings.ContainsAny(s, string(ReservedChars)) {
			return false
		}
	}
	_, err := path.Parser{}.Parse(Leaf{Segments: segs}.Path())
	return err == nil
}

// Values classifies and compares decoded JSON values.
type Values struct{}

// Kind returns the JSON kind of a decoded value.
func (Values) Kind(v any) JSONKind {
	switch v.(type) {
	case string:
		return KindString
	case json.Number, float64:
		return KindNumber
	case bool:
		return KindBool
	case []any:
		return KindArray
	case map[string]any:
		return KindObject
	default:
		return KindNull
	}
}

// Key is a canonical text of a value for equality (numbers compare by value).
func (vs Values) Key(v any) string {
	if n, ok := v.(json.Number); ok {
		if f, err := n.Float64(); err == nil {
			raw, _ := json.Marshal(f)
			return string(KindNumber) + string(raw)
		}
	}
	raw, _ := json.Marshal(v)
	return string(vs.Kind(v)) + string(raw)
}
