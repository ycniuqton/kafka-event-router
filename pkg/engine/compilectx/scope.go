// Package compilectx holds the state shared by filter and step compilers while one
// route version compiles: issue collection, path parsing, script modules, library port.
package compilectx

import (
	"context"
	"errors"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/path"
	"github.com/eventrouter/router/pkg/engine/script"
)

// Scope collects issues and owns resources created during one compile.
type Scope struct {
	Ctx          context.Context
	Issues       config.Issues
	Scripts      script.Runtime
	ScriptLimits script.Limits
	Modules      *script.ModuleSet
	Library      config.LibraryResolver
	Helpers      ctxhelpers.Set
	Limits       config.ResolvedLimits
	Tables       *ctxhelpers.TableLookupStore
	Registries   codec.RegistryProvider
}

// Fail records an error issue at ptr.
func (s *Scope) Fail(code config.IssueCode, ptr config.Pointer, format string, args ...any) {
	s.Issues.Add(config.NewError(code, ptr, format, args...))
}

// Path parses a field path, recording PathInvalid at ptr on failure.
func (s *Scope) Path(raw string, ptr config.Pointer) (path.FieldPath, bool) {
	p, err := path.Parser{}.Parse(raw)
	if err != nil {
		s.Fail(config.IssuePathInvalid, ptr, "%v", err)
		return path.FieldPath{}, false
	}
	return p, true
}

// Script compiles user code bound to the route lookup tables, recording JSCompile at ptr
// on failure; modules are owned by the scope.
func (s *Scope) Script(src script.Source, ptr config.Pointer) (script.Module, bool) {
	if s.Scripts == nil {
		s.Fail(config.IssueJSCompile, ptr, "%v", script.ErrUnavailable)
		return nil, false
	}
	src.Lookups = s.Helpers.Lookup
	m, err := s.Scripts.Compile(s.Ctx, src, s.ScriptLimits)
	if err != nil {
		var ce *script.CompileError
		if errors.As(err, &ce) {
			s.Fail(config.IssueJSCompile, ptr, "%s", ce.Error())
			return nil, false
		}
		s.Fail(config.IssueJSCompile, ptr, "%v", err)
		return nil, false
	}
	s.Modules.Add(m)
	return m, true
}

// Failed reports whether any issue was recorded.
func (s *Scope) Failed() bool { return len(s.Issues) > 0 }
