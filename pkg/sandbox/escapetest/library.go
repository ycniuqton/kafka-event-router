package escapetest

import (
	"context"
	"fmt"

	"github.com/eventrouter/router/pkg/sandbox"
)

// LibraryCode is a library transform that tries to keep state and poison shared intrinsics.
const LibraryCode = `globalThis.loads = (globalThis.loads || 0) + 1;
function transform(event, ctx) {
  const seen = [typeof globalThis.mark, ({}).poisoned === undefined, globalThis.loads];
  globalThis.mark = event.offset; Object.prototype.poisoned = true;
  return { value: seen };
}`

// cleanState is what LibraryCode must observe on every call: no mark, no poison, loaded once.
var cleanState = Returns{Value: []any{"undefined", true, 1.0}}

// LibraryScenario is one way library-sourced JS is shared (plan JS-206).
type LibraryScenario struct {
	Name   string
	Scopes []sandbox.RouteScope
	Paths  []string
}

// LibraryProbe runs LibraryCode through every placement of a scenario and checks that no call
// observes state left by another call, version, branch or route sharing the same compiled program.
type LibraryProbe struct {
	Manager sandbox.Manager
}

// Run returns the first violation of scenario s.
func (p LibraryProbe) Run(ctx context.Context, s LibraryScenario) error {
	var modules []sandbox.Module
	for _, scope := range s.Scopes {
		srcs := make([]sandbox.Source, 0, len(s.Paths))
		for _, path := range s.Paths {
			srcs = append(srcs, sandbox.Source{StepPath: path, Entry: sandbox.EntryTransform, Code: LibraryCode})
		}
		rs, err := p.Manager.Load(ctx, scope, srcs, sandbox.RouteLimits{})
		if err != nil {
			return fmt.Errorf("load %s/%s: %w", scope.RouteID, scope.VersionID, err)
		}
		defer rs.Close(ctx)
		for _, path := range s.Paths {
			mod, _ := rs.Module(path)
			modules = append(modules, mod)
		}
	}
	return p.probe(ctx, modules)
}

// probe calls every module twice, interleaved, expecting a clean state each time.
func (p LibraryProbe) probe(ctx context.Context, modules []sandbox.Module) error {
	for round := range int64(2) {
		for i, mod := range modules {
			res, err := mod.Call(ctx, Event{}.At(round*int64(len(modules))+int64(i)+1))
			if verr := cleanState.Verify(res, err); verr != nil {
				return fmt.Errorf("module %d round %d: %w", i, round, verr)
			}
		}
	}
	return nil
}
