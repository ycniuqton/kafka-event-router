package ctxhelpers

import (
	"fmt"
	"strings"

	"github.com/eventrouter/router/pkg/config"
)

// Masker hides a value according to a mask style; the result is always a string (nil stays nil).
type Masker interface {
	Mask(v any, style config.MaskStyle) (any, error)
}

// MaskPolicy holds the masking symbol, the visible tail length of last4 and the visible head of email local parts.
type MaskPolicy struct {
	Symbol           rune
	VisibleTail      int
	EmailAt          string
	EmailVisibleHead int
}

// DefaultMaskPolicy masks with '*', keeps 4 trailing characters for last4 and the first letter of emails (j***@mail.com).
func DefaultMaskPolicy() MaskPolicy {
	return MaskPolicy{Symbol: '*', VisibleTail: 4, EmailAt: "@", EmailVisibleHead: 1}
}

// MaskStrategy implements one mask style.
type MaskStrategy interface {
	Style() config.MaskStyle
	Apply(s string, p MaskPolicy) string
}

// StyleMasker selects a MaskStrategy by style (registry, no type switch on styles).
type StyleMasker struct {
	policy     MaskPolicy
	strategies map[config.MaskStyle]MaskStrategy
}

// NewStyleMasker registers the full, last4 and email strategies.
func NewStyleMasker(p MaskPolicy) *StyleMasker {
	m := &StyleMasker{policy: p, strategies: map[config.MaskStyle]MaskStrategy{}}
	for _, s := range []MaskStrategy{FullMask{}, Last4Mask{}, EmailMask{}} {
		m.strategies[s.Style()] = s
	}
	return m
}

// Styles lists the registered styles (engine catalogue).
func (m *StyleMasker) Styles() []config.MaskStyle {
	return []config.MaskStyle{config.MaskStyleFull, config.MaskStyleLast4, config.MaskStyleEmail}
}

// Mask applies style to the textual form of v.
func (m *StyleMasker) Mask(v any, style config.MaskStyle) (any, error) {
	if v == nil {
		return nil, nil
	}
	s, ok := m.strategies[style]
	if !ok {
		return nil, fmt.Errorf("unknown mask style %q", style)
	}
	return s.Apply(TextForm{}.Of(v), m.policy), nil
}

// FullMask replaces every character.
type FullMask struct{}

func (FullMask) Style() config.MaskStyle { return config.MaskStyleFull }

func (FullMask) Apply(s string, p MaskPolicy) string {
	return strings.Repeat(string(p.Symbol), len([]rune(s)))
}

// Last4Mask keeps the last VisibleTail characters.
type Last4Mask struct{}

func (Last4Mask) Style() config.MaskStyle { return config.MaskStyleLast4 }

func (Last4Mask) Apply(s string, p MaskPolicy) string {
	r := []rune(s)
	if len(r) <= p.VisibleTail {
		return strings.Repeat(string(p.Symbol), len(r))
	}
	cut := len(r) - p.VisibleTail
	return strings.Repeat(string(p.Symbol), cut) + string(r[cut:])
}

// EmailMask keeps the first EmailVisibleHead letters of the local part and the domain (john@mail.com →
// j***@mail.com, UI spec §8.4); local parts no longer than the head and non-emails are fully masked.
type EmailMask struct{}

func (EmailMask) Style() config.MaskStyle { return config.MaskStyleEmail }

func (EmailMask) Apply(s string, p MaskPolicy) string {
	at := strings.LastIndex(s, p.EmailAt)
	if at <= 0 {
		return FullMask{}.Apply(s, p)
	}
	local := []rune(s[:at])
	if len(local) <= p.EmailVisibleHead {
		return FullMask{}.Apply(s[:at], p) + s[at:]
	}
	head := string(local[:p.EmailVisibleHead])
	return head + FullMask{}.Apply(string(local[p.EmailVisibleHead:]), p) + s[at:]
}

// TextForm renders scalars as text for masking and hashing.
type TextForm struct{}

// Of returns strings unchanged, bytes as text and everything else via fmt.
func (TextForm) Of(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	}
	return fmt.Sprint(v)
}
