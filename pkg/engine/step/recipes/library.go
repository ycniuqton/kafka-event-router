package recipes

import (
	"errors"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compilectx"
	"github.com/eventrouter/router/pkg/engine/step"
	"github.com/eventrouter/router/pkg/engine/trace"
)

// Library expands a pinned library transform inline at compile time: its enabled steps
// join the parent chain with ids `<stepId>/<ref>@<version>/<childId>`.
type Library struct{}

func (Library) Kind() config.StepKind { return config.StepKindLibrary }
func (Library) Group() step.Group     { return step.GroupLibrary }

func (Library) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	pin, ok := LibraryPins{}.Of(c, raw)
	if !ok {
		return nil, false
	}
	steps, ok := LibraryTransforms{}.Load(c, pin)
	if !ok {
		return nil, false
	}
	return LibraryExpander{}.Expand(c, raw, pin, steps)
}

// LibraryPins reads the `ref` + `version` pin of a library-backed step.
type LibraryPins struct{}

// Of returns the pin, recording issues when it is incomplete or no resolver is configured.
func (LibraryPins) Of(c *step.StepCompileCtx, raw config.Step) (compilectx.LibraryPin, bool) {
	if raw.Ref == nil || raw.Version == nil {
		c.Fail(config.IssueSchemaInvalid, config.TokenRef, "%s steps require ref and version", raw.Kind)
		return compilectx.LibraryPin{}, false
	}
	pin := compilectx.LibraryPin{Ref: *raw.Ref, Version: *raw.Version}
	if c.Scope.Library == nil {
		c.Fail(config.IssueLibraryRefMissing, config.TokenRef, "%s: no library is available to resolve this reference", pin)
		return pin, false
	}
	return pin, true
}

// LibraryErrors maps resolver failures to LibraryRefMissing issues on the step's ref.
type LibraryErrors struct{}

// Fail records err for pin; what names the library object (transform, lookup table).
func (LibraryErrors) Fail(c *step.StepCompileCtx, pin compilectx.LibraryPin, what LibraryObject, err error) {
	if errors.Is(err, config.ErrLibraryRefMissing) {
		c.Fail(config.IssueLibraryRefMissing, config.TokenRef, "library %s %s does not exist (unknown name or deleted version)", what, pin)
		return
	}
	c.Fail(config.IssueLibraryRefMissing, config.TokenRef, "library %s %s could not be loaded: %v", what, pin, err)
}

// LibraryObject names the kinds of library objects in messages.
type LibraryObject string

const (
	LibraryObjectTransform   LibraryObject = "transform"
	LibraryObjectLookupTable LibraryObject = "lookup table"
)

// LibraryTransforms loads pinned transforms, refusing cycles and over-deep nesting.
type LibraryTransforms struct{}

// Load returns the pinned steps or records an issue.
func (LibraryTransforms) Load(c *step.StepCompileCtx, pin compilectx.LibraryPin) (config.Steps, bool) {
	if c.Libraries.Contains(pin.Ref) {
		c.Fail(config.IssueLibraryRefMissing, config.TokenRef, "library transform %s includes itself", pin)
		return nil, false
	}
	if c.Libraries.Full() {
		c.Fail(config.IssueLibraryRefMissing, config.TokenRef, "library transform %s nests more than %d library levels", pin, step.LibraryMaxDepth)
		return nil, false
	}
	steps, err := c.Scope.Library.Transform(c.Scope.Ctx, pin.Ref, pin.Version)
	if err != nil {
		LibraryErrors{}.Fail(c, pin, LibraryObjectTransform, err)
		return nil, false
	}
	return steps, true
}

// LibraryExpander compiles pinned steps in an isolated scope and re-reports their issues
// at the library step, since the steps are not part of the route document.
type LibraryExpander struct{}

// Expand compiles steps into spliceable entries addressed by the library step pointer.
func (LibraryExpander) Expand(c *step.StepCompileCtx, raw config.Step, pin compilectx.LibraryPin, steps config.Steps) (step.Step, bool) {
	scope := *c.Scope
	scope.Issues = nil
	sub := *c
	sub.Scope, sub.Libraries = &scope, c.Libraries.Push(pin.Ref)
	prefix := c.ID + step.NestedIDSeparator + pin.String() + step.NestedIDSeparator
	chain := sub.Chain(config.PointerRoot, prefix, steps)
	for _, is := range scope.Issues {
		c.Scope.Issues.Add(config.Issue{Code: is.Code, Severity: is.Severity, Path: c.Pointer,
			Message: "library transform " + pin.String() + " step " + string(is.Path) + ": " + is.Message})
	}
	entries := make([]step.ChainEntry, 0, chain.Len())
	for _, e := range chain.Entries() {
		entries = append(entries, step.ChainEntry{Step: e.Step, Pointer: c.Pointer})
	}
	return &library{Base: step.NewBase(c, raw), chain: chain, entries: entries}, len(scope.Issues) == 0
}

// library runs its expanded chain when not spliced (nested contexts) and splices otherwise.
type library struct {
	step.Base
	chain   *step.StepChain
	entries []step.ChainEntry
}

func (s *library) Entries() []step.ChainEntry { return s.entries }

func (s *library) Apply(sc *step.StepContext, in engine.Event) ([]engine.Event, error) {
	return s.chain.Run(sc, trace.NoopTracer{}, in)
}
