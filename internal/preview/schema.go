package preview

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/eventrouter/router/pkg/config"
)

// FieldPathSyntax names the separators of dotted field paths ("value.items[].sku").
type FieldPathSyntax string

const (
	PathSeparator FieldPathSyntax = "."
	PathArrayItem FieldPathSyntax = "[]"
)

// ValueCount is one frequent value of a low-cardinality field.
type ValueCount struct {
	Value any     `json:"value"`
	Count int     `json:"count"`
	Pct   float64 `json:"pct"`
}

// FieldNode is one field of the inferred schema tree; Registry marks fields declared by the registry schema.
type FieldNode struct {
	Path      string           `json:"path"`
	Types     map[JSONType]int `json:"types"`
	Present   int              `json:"present"`
	Frequency float64          `json:"frequency"`
	TopValues []ValueCount     `json:"topValues"`
	Children  []FieldNode      `json:"children"`
	Registry  bool             `json:"registry,omitempty"`
}

// FieldSchema is the inferred schema of a session's decoded events in Format (stale formats are re-inferred).
type FieldSchema struct {
	Samples int               `json:"samples"`
	Root    FieldNode         `json:"root"`
	Format  config.FormatType `json:"format,omitempty"`
}

// SchemaInferrer builds a FieldSchema from event documents ({key, value, headers}).
type SchemaInferrer struct {
	TopValues      int
	LowCardinality int
	MaxDepth       int
}

// NewSchemaInferrer returns the inferrer with the domain limits.
func NewSchemaInferrer() SchemaInferrer {
	return SchemaInferrer{TopValues: int(SchemaTopValues), LowCardinality: int(SchemaLowCardinality), MaxDepth: int(SchemaMaxDepth)}
}

// Infer aggregates every document into the schema tree.
func (s SchemaInferrer) Infer(docs []any) FieldSchema {
	root := newFieldAcc("", s.LowCardinality)
	for _, d := range docs {
		root.visit(d, 0, s.MaxDepth)
	}
	return FieldSchema{Samples: len(docs), Root: root.node(len(docs), s.TopValues)}
}

// fieldAcc accumulates the observations of one path.
type fieldAcc struct {
	path     string
	limit    int
	types    map[JSONType]int
	present  int
	values   map[string]*ValueCount
	overflow bool
	children map[string]*fieldAcc
	order    []string
}

func newFieldAcc(path string, limit int) *fieldAcc {
	return &fieldAcc{path: path, limit: limit, types: map[JSONType]int{}, values: map[string]*ValueCount{}, children: map[string]*fieldAcc{}}
}

// visit records v at this path and descends into objects and arrays.
func (a *fieldAcc) visit(v any, depth, maxDepth int) {
	a.present++
	t := JSONValues{}.TypeOf(v)
	a.types[t]++
	if depth >= maxDepth {
		return
	}
	switch n := v.(type) {
	case map[string]any:
		for _, k := range (JSONValues{}).SortedKeys(n) {
			a.child(k, string(PathSeparator)).visit(n[k], depth+1, maxDepth)
		}
	case []any:
		for _, item := range n {
			a.child(string(PathArrayItem), "").visit(item, depth+1, maxDepth)
		}
	default:
		a.count(v)
	}
}

func (a *fieldAcc) child(name, sep string) *fieldAcc {
	c, ok := a.children[name]
	if ok {
		return c
	}
	path := name
	if a.path != "" {
		path = a.path + sep + name
	}
	c = newFieldAcc(path, a.limit)
	a.children[name] = c
	a.order = append(a.order, name)
	return c
}

// count tallies a scalar until the field proves high-cardinality.
func (a *fieldAcc) count(v any) {
	if a.overflow {
		return
	}
	key := fmt.Sprintf("%T:%v", v, v)
	if vc, ok := a.values[key]; ok {
		vc.Count++
		return
	}
	if len(a.values) >= a.limit {
		a.overflow, a.values = true, nil
		return
	}
	a.values[key] = &ValueCount{Value: v, Count: 1}
}

// node converts the accumulator to a FieldNode; frequency is relative to the sample count.
func (a *fieldAcc) node(samples, top int) FieldNode {
	n := FieldNode{Path: a.path, Types: a.types, Present: a.present, TopValues: a.top(top), Children: []FieldNode{}}
	if samples > 0 {
		n.Frequency = min(1, float64(a.present)/float64(samples))
	}
	for _, name := range a.order {
		n.Children = append(n.Children, a.children[name].node(samples, top))
	}
	return n
}

func (a *fieldAcc) top(limit int) []ValueCount {
	out := make([]ValueCount, 0, len(a.values))
	for _, vc := range a.values {
		c := *vc
		c.Pct = float64(c.Count) / float64(a.present)
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return fmt.Sprint(out[i].Value) < fmt.Sprint(out[j].Value)
	})
	return out[:min(limit, len(out))]
}

// JSONValues classifies and walks decoded JSON values.
type JSONValues struct{}

// TypeOf classifies a decoded JSON value.
func (JSONValues) TypeOf(v any) JSONType {
	switch v.(type) {
	case nil:
		return TypeNull
	case string:
		return TypeString
	case bool:
		return TypeBoolean
	case map[string]any:
		return TypeObject
	case []any:
		return TypeArray
	case float64, float32, int, int32, int64, json.Number:
		return TypeNumber
	}
	return TypeString
}

// SortedKeys returns the keys of m in order (deterministic schemas and diffs).
func (JSONValues) SortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
