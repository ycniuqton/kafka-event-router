package enrich

import (
	"errors"
	"fmt"
	"strings"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/path"
	"github.com/eventrouter/router/pkg/engine/step"
)

// PickSyntax is the JSONPath-ish grammar of response picks.
type PickSyntax string

const (
	PickRoot   PickSyntax = "$"
	PickMember PickSyntax = "."
)

// ErrPick reports a malformed response pick.
var ErrPick = errors.New("invalid response pick")

// ResponsePick selects a value from an answer: `$`, `$.a.b`, `$.items[0].name`, `$[0]`.
type ResponsePick struct {
	whole bool
	field path.FieldPath
}

// PickParser parses response picks onto engine field paths rooted at value.
type PickParser struct{}

// Parse validates the `$` root and the rest of the path.
func (PickParser) Parse(raw string) (ResponsePick, error) {
	if !strings.HasPrefix(raw, string(PickRoot)) {
		return ResponsePick{}, fmt.Errorf("%w %q: must start with $", ErrPick, raw)
	}
	rest := strings.TrimPrefix(raw, string(PickRoot))
	if rest == "" {
		return ResponsePick{whole: true}, nil
	}
	p, err := path.Parser{}.Parse(string(path.RootValue) + rest)
	if err != nil {
		return ResponsePick{}, fmt.Errorf("%w %q: %v", ErrPick, raw, err)
	}
	return ResponsePick{field: p}, nil
}

// Pick returns the selected value of answer; ok is false when it does not exist.
func (p ResponsePick) Pick(answer any) (any, bool) {
	if p.whole {
		return answer, answer != nil
	}
	ev := engine.Event{Value: answer}
	return p.field.Get(&ev)
}

// Mapping writes one picked value to one target field.
type Mapping struct {
	from ResponsePick
	to   step.FieldRef
}

// Mappings is the ordered `map` of a lookup step.
type Mappings []Mapping

// MappingCompiler compiles the `map` member.
type MappingCompiler struct{}

// Compile parses every from/to pair, recording issues under ptr.
func (MappingCompiler) Compile(c *step.StepCompileCtx, ptr config.Pointer, raw []config.LookupMapping) (Mappings, bool) {
	if len(raw) == 0 {
		c.Scope.Fail(config.IssueSchemaInvalid, ptr, "map needs at least one from → to pair")
		return nil, false
	}
	out := make(Mappings, 0, len(raw))
	ok := true
	for i, m := range raw {
		at := ptr.Index(i)
		pick, err := PickParser{}.Parse(m.From)
		if err != nil {
			c.Scope.Fail(config.IssuePathInvalid, at.Token(config.TokenFrom), "%v", err)
			ok = false
		}
		to, okT := c.Scope.Path(m.To, at.Token(config.TokenTo))
		ok = ok && okT
		out = append(out, Mapping{from: pick, to: step.FieldRef{FieldPath: to}})
	}
	return out, ok
}

// Apply writes every picked value of answer; picks that select nothing leave their target unchanged.
func (ms Mappings) Apply(ev *engine.Event, answer any) error {
	for _, m := range ms {
		v, ok := m.from.Pick(answer)
		if !ok {
			continue
		}
		if err := m.to.Write(ev, engine.ValueCloner{}.Clone(v)); err != nil {
			return err
		}
	}
	return nil
}

// Fill writes v to every target (onMiss / onError `default`).
func (ms Mappings) Fill(ev *engine.Event, v any) error {
	for _, m := range ms {
		if err := m.to.Write(ev, engine.ValueCloner{}.Clone(v)); err != nil {
			return err
		}
	}
	return nil
}

// Targets returns the written field paths.
func (ms Mappings) Targets() []path.FieldPath {
	out := make([]path.FieldPath, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.to.FieldPath)
	}
	return out
}
