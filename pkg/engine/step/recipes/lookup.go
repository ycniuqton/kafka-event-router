package recipes

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/enrich"
	"github.com/eventrouter/router/pkg/engine/formula"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Lookup maps the text of `field` through inline rows or a pinned library table into
// `to` (default `field`); the table is also registered for formula lookup() and ctx.lookup.
// With `lookup` set the step calls a managed lookup source instead (enrich.Compiler).
type Lookup struct{}

func (Lookup) Kind() config.StepKind { return config.StepKindLookup }
func (Lookup) Group() step.Group     { return step.GroupValues }

func (Lookup) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	if raw.Lookup != nil {
		return enrich.DefaultCompiler().Compile(c, raw)
	}
	rows, okR := LookupTables{}.Load(c, raw)
	source, okF := c.Path(raw.Field, config.TokenField)
	target, okT := source, true
	if raw.To != nil {
		target, okT = c.Path(raw.To, config.TokenTo)
	}
	s := &lookup{Base: step.NewBase(c, raw), source: source, target: target, rows: rows}
	if raw.DefaultValue != nil {
		s.fallback, s.hasFallback = step.NumberNormalizer{}.Of(raw.DefaultValue), true
	}
	return s, okR && okF && okT
}

// LookupTables loads the table of a lookup step and registers it in the route scope:
// inline rows under the step id; library rows also under `<ref>@<version>` and `<ref>`.
type LookupTables struct{}

// Load returns key → value rows or records an issue.
func (LookupTables) Load(c *step.StepCompileCtx, raw config.Step) (map[string]any, bool) {
	if raw.Rows != nil {
		rows := LookupRows{}.Map(raw.Rows)
		c.Scope.Tables.Register(c.ID, rows)
		return rows, true
	}
	pin, ok := LibraryPins{}.Of(c, raw)
	if !ok {
		return nil, false
	}
	loaded, err := c.Scope.Library.LookupTable(c.Scope.Ctx, pin.Ref, pin.Version)
	if err != nil {
		LibraryErrors{}.Fail(c, pin, LibraryObjectLookupTable, err)
		return nil, false
	}
	rows := LookupRows{}.Map(loaded)
	c.Scope.Tables.Register(c.ID, rows)
	c.Scope.Tables.Register(pin.String(), rows)
	c.Scope.Tables.RegisterAlias(pin.Ref, pin.String())
	return rows, true
}

// LookupRows converts document rows into a key map (last row wins on duplicates).
type LookupRows struct{}

// Map returns key → normalised value.
func (LookupRows) Map(rows config.LookupRows) map[string]any {
	out := make(map[string]any, len(rows))
	for _, r := range rows {
		out[r.Key] = step.NumberNormalizer{}.Of(r.Value)
	}
	return out
}

type lookup struct {
	step.Base
	source, target step.FieldRef
	rows           map[string]any
	fallback       any
	hasFallback    bool
}

func (s *lookup) Apply(_ *step.StepContext, in engine.Event) ([]engine.Event, error) {
	k, ok := s.source.Get(&in)
	if !ok {
		return step.One(in), s.Missing(s.source)
	}
	v, hit := s.rows[formula.Text{}.Of(k)]
	if !hit && !s.hasFallback {
		return step.One(in), nil
	}
	if !hit {
		v = s.fallback
	}
	if err := s.target.Write(&in, engine.ValueCloner{}.Clone(v)); err != nil {
		return nil, err
	}
	return step.One(in), nil
}
