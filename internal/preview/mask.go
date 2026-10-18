package preview

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// MaskToken holds the replacement texts maskers write instead of real values.
type MaskToken string

const (
	MaskedString   MaskToken = "••••••"
	RedactedValue  MaskToken = "[redacted]"
	PartialFill    MaskToken = "•"
	PartialKeepMin MaskToken = "••••"
)

// PartialPolicy bounds how much of a string the partial masker reveals.
type PartialPolicy int

const (
	PartialRevealEdge PartialPolicy = 1
	PartialMinLength  PartialPolicy = 6
)

// ValueMasker replaces every leaf of a decoded JSON value; object keys and array shape are kept.
type ValueMasker interface {
	Style() MaskStyle
	Mask(v any) any
	MaskText(s string) string
}

// LeafRule masks one scalar leaf; the shared walker applies it to every leaf of a value.
type LeafRule interface {
	String(s string) any
	Number() any
	Bool() any
}

// TreeMasker is the base of every masker: it walks maps, arrays and raw JSON and masks leaves by rule.
type TreeMasker struct {
	style MaskStyle
	rule  LeafRule
}

func (m TreeMasker) Style() MaskStyle { return m.style }

// MaskText masks a free text (raw bytes, header values, logs) as a string leaf.
func (m TreeMasker) MaskText(s string) string {
	if out, ok := m.rule.String(s).(string); ok {
		return out
	}
	return string(MaskedString)
}

// Mask returns a masked copy of v; nil stays nil so presence checks keep working.
func (m TreeMasker) Mask(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			out[k] = m.Mask(child)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = m.Mask(child)
		}
		return out
	case json.RawMessage:
		return m.raw(t)
	}
	return m.leaf(v)
}

// raw decodes embedded JSON before masking (output values travel as json.RawMessage).
func (m TreeMasker) raw(b json.RawMessage) any {
	var decoded any
	if json.Unmarshal(b, &decoded) != nil {
		return m.MaskText(string(b))
	}
	return m.Mask(decoded)
}

func (m TreeMasker) leaf(v any) any {
	switch t := v.(type) {
	case string:
		return m.rule.String(t)
	case bool:
		return m.rule.Bool()
	case float64, float32, int, int32, int64, uint, uint32, uint64, json.Number:
		return m.rule.Number()
	}
	return m.rule.String("")
}

// TypePreservingRule keeps JSON types: strings → masked string, numbers → 0, booleans → false.
type TypePreservingRule struct{}

func (TypePreservingRule) String(string) any { return string(MaskedString) }
func (TypePreservingRule) Number() any       { return 0 }
func (TypePreservingRule) Bool() any         { return false }

// RedactRule replaces every leaf by one redaction marker string.
type RedactRule struct{}

func (RedactRule) String(string) any { return string(RedactedValue) }
func (RedactRule) Number() any       { return string(RedactedValue) }
func (RedactRule) Bool() any         { return string(RedactedValue) }

// PartialRule keeps the first and last character of long strings; short strings and other leaves are hidden.
type PartialRule struct{}

func (PartialRule) String(s string) any {
	n := utf8.RuneCountInString(s)
	if n < int(PartialMinLength) {
		return string(PartialKeepMin)
	}
	runes := []rune(s)
	edge := int(PartialRevealEdge)
	return string(runes[:edge]) + strings.Repeat(string(PartialFill), n-2*edge) + string(runes[n-edge:])
}
func (PartialRule) Number() any { return 0 }
func (PartialRule) Bool() any   { return false }

// NewTypePreservingMasker is the default masker (decision 3).
func NewTypePreservingMasker() TreeMasker {
	return TreeMasker{style: MaskTypePreserving, rule: TypePreservingRule{}}
}

// NewRedactMasker hides every leaf behind one marker.
func NewRedactMasker() TreeMasker { return TreeMasker{style: MaskRedact, rule: RedactRule{}} }

// NewPartialMasker reveals only the edges of long strings.
func NewPartialMasker() TreeMasker { return TreeMasker{style: MaskPartial, rule: PartialRule{}} }

// MaskerManager selects maskers by style; business logic only talks to it.
type MaskerManager struct {
	maskers      map[MaskStyle]ValueMasker
	defaultStyle MaskStyle
}

// NewMaskerManager registers maskers; unknown or empty styles fall back to defaultStyle.
func NewMaskerManager(defaultStyle MaskStyle, maskers ...ValueMasker) *MaskerManager {
	m := &MaskerManager{maskers: map[MaskStyle]ValueMasker{}, defaultStyle: defaultStyle}
	for _, v := range maskers {
		m.maskers[v.Style()] = v
	}
	return m
}

// DefaultMaskerManager registers the three styles with type-preserving as default.
func DefaultMaskerManager() *MaskerManager {
	return NewMaskerManager(MaskTypePreserving, NewTypePreservingMasker(), NewRedactMasker(), NewPartialMasker())
}

// For returns the masker of style (default for unknown styles).
func (m *MaskerManager) For(style MaskStyle) ValueMasker {
	if v, ok := m.maskers[style]; ok {
		return v
	}
	return m.maskers[m.defaultStyle]
}
