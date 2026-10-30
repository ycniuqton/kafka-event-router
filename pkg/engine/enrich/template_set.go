package enrich

import (
	"fmt"
	"sort"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/path"
)

// TemplateMap is a named set of templates (HTTP query/headers, SQL params).
type TemplateMap map[string]Template

// TemplateMapParser parses every value of a raw map.
type TemplateMapParser struct{}

// Parse returns the compiled map or the first failing name.
func (TemplateMapParser) Parse(raw map[string]string) (TemplateMap, string, error) {
	out := make(TemplateMap, len(raw))
	for _, name := range (SortedNames{}).Of(raw) {
		t, err := TemplateParser{}.Parse(raw[name])
		if err != nil {
			return nil, name, err
		}
		out[name] = t
	}
	return out, "", nil
}

// Text renders every entry as text; ok is false when a field is missing.
func (m TemplateMap) Text(ev *engine.Event) (map[string]string, bool) {
	if len(m) == 0 {
		return nil, true
	}
	out := make(map[string]string, len(m))
	for name, t := range m {
		v, ok := t.Text(ev, PlainText{})
		if !ok {
			return nil, false
		}
		out[name] = v
	}
	return out, true
}

// Fields returns every placeholder path of the map.
func (m TemplateMap) Fields() []path.FieldPath {
	var out []path.FieldPath
	for _, t := range m {
		out = append(out, t.Fields()...)
	}
	return out
}

// SortedNames orders map keys for deterministic compile issues.
type SortedNames struct{}

// Of returns the sorted keys.
func (SortedNames) Of(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TemplateList is an ordered list of templates (Redis MGET keys).
type TemplateList []Template

// TemplateListParser parses a list of raw templates.
type TemplateListParser struct{}

// Parse returns the list or the failing index.
func (TemplateListParser) Parse(raw []string) (TemplateList, int, error) {
	out := make(TemplateList, 0, len(raw))
	for i, r := range raw {
		t, err := TemplateParser{}.Parse(r)
		if err != nil {
			return nil, i, err
		}
		out = append(out, t)
	}
	return out, -1, nil
}

// Text renders every template; ok is false when a field is missing.
func (l TemplateList) Text(ev *engine.Event) ([]string, bool) {
	out := make([]string, 0, len(l))
	for _, t := range l {
		v, ok := t.Text(ev, PlainText{})
		if !ok {
			return nil, false
		}
		out = append(out, v)
	}
	return out, true
}

// Fields returns every placeholder path of the list.
func (l TemplateList) Fields() []path.FieldPath {
	var out []path.FieldPath
	for _, t := range l {
		out = append(out, t.Fields()...)
	}
	return out
}

// BodyTemplate is a JSON body whose string leaves are templates (typed when a single placeholder).
type BodyTemplate struct {
	constant any
	text     *Template
	object   map[string]BodyTemplate
	array    []BodyTemplate
}

// BodyTemplateParser compiles a document body.
type BodyTemplateParser struct{}

// Parse walks the body; constants are normalised document values.
func (p BodyTemplateParser) Parse(raw any) (BodyTemplate, error) {
	switch t := raw.(type) {
	case string:
		tpl, err := TemplateParser{}.Parse(t)
		return BodyTemplate{text: &tpl}, err
	case map[string]any:
		return p.object(t)
	case []any:
		return p.array(t)
	}
	return BodyTemplate{constant: raw}, nil
}

func (p BodyTemplateParser) object(raw map[string]any) (BodyTemplate, error) {
	out := BodyTemplate{object: make(map[string]BodyTemplate, len(raw))}
	for k, v := range raw {
		child, err := p.Parse(v)
		if err != nil {
			return BodyTemplate{}, fmt.Errorf("%s: %w", k, err)
		}
		out.object[k] = child
	}
	return out, nil
}

func (p BodyTemplateParser) array(raw []any) (BodyTemplate, error) {
	out := BodyTemplate{array: make([]BodyTemplate, 0, len(raw))}
	for i, v := range raw {
		child, err := p.Parse(v)
		if err != nil {
			return BodyTemplate{}, fmt.Errorf("[%d]: %w", i, err)
		}
		out.array = append(out.array, child)
	}
	return out, nil
}

// Render builds the body for one event; ok is false when a field is missing.
func (b BodyTemplate) Render(ev *engine.Event) (any, bool) {
	switch {
	case b.text != nil:
		return b.text.Value(ev)
	case b.object != nil:
		return b.renderObject(ev)
	case b.array != nil:
		return b.renderArray(ev)
	}
	return engine.ValueCloner{}.Clone(b.constant), true
}

func (b BodyTemplate) renderObject(ev *engine.Event) (any, bool) {
	out := make(map[string]any, len(b.object))
	for k, child := range b.object {
		v, ok := child.Render(ev)
		if !ok {
			return nil, false
		}
		out[k] = v
	}
	return out, true
}

func (b BodyTemplate) renderArray(ev *engine.Event) (any, bool) {
	out := make([]any, 0, len(b.array))
	for _, child := range b.array {
		v, ok := child.Render(ev)
		if !ok {
			return nil, false
		}
		out = append(out, v)
	}
	return out, true
}

// Fields returns every placeholder path of the body.
func (b BodyTemplate) Fields() []path.FieldPath {
	var out []path.FieldPath
	if b.text != nil {
		out = append(out, b.text.Fields()...)
	}
	for _, child := range b.object {
		out = append(out, child.Fields()...)
	}
	for _, child := range b.array {
		out = append(out, child.Fields()...)
	}
	return out
}
