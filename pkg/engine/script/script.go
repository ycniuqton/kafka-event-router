// Package script is the engine's port to a JavaScript sandbox: the engine compiles
// `js` steps and `expr` filter nodes through Runtime and calls Modules per event.
// pkg/sandbox (js-sandbox) provides the production adapter.
package script

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// EntryKind is the contract the user code must satisfy.
type EntryKind string

const (
	EntryTransform EntryKind = "transform"
	EntryPredicate EntryKind = "predicate"
)

// ErrTimeout marks a call that exceeded its wall-clock limit (mapped to step_timeout).
var ErrTimeout = errors.New("script timeout")

// ErrUnavailable is returned when no script runtime is configured.
var ErrUnavailable = errors.New("script runtime unavailable")

// Source is one piece of user code, where it lives in the route document and the
// lookup tables pinned by its route version (ctx.lookup reads them).
type Source struct {
	Code    string
	Entry   EntryKind
	Origin  string
	Lookups ctxhelpers.LookupStore
}

// Limits bounds one call.
type Limits struct {
	Timeout        time.Duration
	MemoryBytes    int64
	MaxOutputBytes int64
}

// Invocation is one call: the event in its JSON shape and the frozen processing time.
type Invocation struct {
	Event map[string]any
	Now   time.Time
}

// Result is the returned value (event, array of events, null or boolean) plus buffered logs.
type Result struct {
	Value any
	Logs  []string
}

// CompileError is a syntax or contract error with an optional source position.
type CompileError struct {
	Message string
	Line    int
	Column  int
}

func (e *CompileError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%d:%d: %s", e.Line, e.Column, e.Message)
	}
	return e.Message
}

// Module is compiled user code, safe for concurrent calls.
type Module interface {
	Call(ctx context.Context, inv Invocation) (Result, error)
	Close(ctx context.Context) error
}

// Runtime compiles user code into modules.
type Runtime interface {
	Compile(ctx context.Context, src Source, limits Limits) (Module, error)
}

// ModuleSet owns the modules created while compiling one pipeline.
type ModuleSet struct {
	mu      sync.Mutex
	modules []Module
}

// Add registers a module for Close.
func (s *ModuleSet) Add(m Module) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modules = append(s.modules, m)
}

// Close releases every module, returning the first error.
func (s *ModuleSet) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var first error
	for _, m := range s.modules {
		if err := m.Close(ctx); err != nil && first == nil {
			first = err
		}
	}
	s.modules = nil
	return first
}
