// Package enrich implements the source mode of the lookup step: request templates bound from event
// fields, response mapping, miss/error policies and the per-step answer cache. It calls managed lookup
// sources only through the ctxhelpers.SourceGateway port and imports no drivers.
package enrich

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/formula"
	"github.com/eventrouter/router/pkg/engine/path"
)

// TemplateSyntax holds the placeholder delimiters of request templates.
type TemplateSyntax string

const (
	PlaceholderOpen  TemplateSyntax = "${"
	PlaceholderClose TemplateSyntax = "}"
)

// ErrTemplate reports a malformed template.
var ErrTemplate = errors.New("invalid template")

// templatePart is literal text or one field placeholder.
type templatePart struct {
	text    string
	field   path.FieldPath
	isField bool
}

// Template is request text with `${field.path}` placeholders, parsed once at compile time.
type Template struct {
	parts []templatePart
}

// Escaper encodes placeholder values for their position (URL path segment, plain text).
type Escaper interface {
	Escape(s string) string
}

// PlainText leaves values unchanged.
type PlainText struct{}

func (PlainText) Escape(s string) string { return s }

// PathSegment escapes values for an URL path.
type PathSegment struct{}

func (PathSegment) Escape(s string) string { return url.PathEscape(s) }

// TemplateParser parses template strings.
type TemplateParser struct{}

// Parse splits raw into literal text and parsed field placeholders.
func (TemplateParser) Parse(raw string) (Template, error) {
	var t Template
	rest := raw
	for rest != "" {
		open := strings.Index(rest, string(PlaceholderOpen))
		if open < 0 {
			t.parts = append(t.parts, templatePart{text: rest})
			break
		}
		closeAt := strings.Index(rest[open:], string(PlaceholderClose))
		if closeAt < 0 {
			return Template{}, fmt.Errorf("%w %q: unterminated placeholder", ErrTemplate, raw)
		}
		part, err := TemplateParser{}.placeholder(rest[open+len(PlaceholderOpen) : open+closeAt])
		if err != nil {
			return Template{}, fmt.Errorf("%w %q: %v", ErrTemplate, raw, err)
		}
		t.parts = append(t.parts, templatePart{text: rest[:open]}, part)
		rest = rest[open+closeAt+len(PlaceholderClose):]
	}
	return t.compact(), nil
}

func (TemplateParser) placeholder(inner string) (templatePart, error) {
	p, err := path.Parser{}.Parse(strings.TrimSpace(inner))
	if err != nil {
		return templatePart{}, err
	}
	return templatePart{field: p, isField: true}, nil
}

// compact drops empty literal parts.
func (t Template) compact() Template {
	out := Template{}
	for _, p := range t.parts {
		if p.isField || p.text != "" {
			out.parts = append(out.parts, p)
		}
	}
	return out
}

// Single reports whether the template is exactly one placeholder (its value keeps its type).
func (t Template) Single() bool { return len(t.parts) == 1 && t.parts[0].isField }

// Fields returns the placeholder paths in order.
func (t Template) Fields() []path.FieldPath {
	var out []path.FieldPath
	for _, p := range t.parts {
		if p.isField {
			out = append(out, p.field)
		}
	}
	return out
}

// Value renders a single placeholder as its typed value, other templates as text; ok is false when a field is missing.
func (t Template) Value(ev *engine.Event) (any, bool) {
	if t.Single() {
		v, ok := t.parts[0].field.Get(ev)
		return engine.ValueCloner{}.Clone(v), ok && v != nil
	}
	return t.Text(ev, PlainText{})
}

// Text renders the template with every placeholder value as escaped text; ok is false when a field is missing.
func (t Template) Text(ev *engine.Event, esc Escaper) (string, bool) {
	var b strings.Builder
	for _, p := range t.parts {
		if !p.isField {
			b.WriteString(p.text)
			continue
		}
		v, ok := p.field.Get(ev)
		if !ok || v == nil {
			return "", false
		}
		b.WriteString(esc.Escape(formula.Text{}.Of(v)))
	}
	return b.String(), true
}
