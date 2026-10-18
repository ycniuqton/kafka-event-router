package enginebind

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/step/recipes"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// ScopeLookups lists the library lookup tables a route version pins, so ctx.lookup reads the same
// table versions as the lookup recipe.
type ScopeLookups struct{}

// Of returns one pin per `lookup` step with `ref` and `version` (nested if/else steps included).
func (s ScopeLookups) Of(cfg *config.RouteConfig) []host.LookupPin {
	if cfg == nil {
		return nil
	}
	var out []host.LookupPin
	for _, b := range cfg.Branches {
		if b.Transform != nil {
			out = s.collect(b.Transform.Steps, out)
		}
	}
	return out
}

// collect appends the pins of steps and their nested branches.
func (s ScopeLookups) collect(steps config.Steps, out []host.LookupPin) []host.LookupPin {
	for _, st := range steps {
		if pin, ok := s.pinOf(st); ok {
			out = append(out, pin)
		}
		if st.ThenSteps != nil {
			out = s.collect(*st.ThenSteps, out)
		}
		if st.ElseSteps != nil {
			out = s.collect(*st.ElseSteps, out)
		}
	}
	return out
}

// pinOf reads a named, versioned lookup step.
func (ScopeLookups) pinOf(st config.Step) (host.LookupPin, bool) {
	if st.Kind != config.StepKindLookup || st.Ref == nil || st.Version == nil {
		return host.LookupPin{}, false
	}
	return host.LookupPin{Table: *st.Ref, Version: *st.Version}, true
}

// LibraryTables adapts the engine's config.LibraryResolver (transform-studio library store) to the
// sandbox port; rows are normalised exactly like the lookup recipe's.
type LibraryTables struct {
	Resolver config.LibraryResolver
}

// LookupTable loads one pinned table version.
func (l LibraryTables) LookupTable(ctx context.Context, table string, version int) (host.LookupRows, error) {
	rows, err := l.Resolver.LookupTable(ctx, table, version)
	if err != nil {
		return nil, err
	}
	return host.LookupRows(recipes.LookupRows{}.Map(rows)), nil
}
