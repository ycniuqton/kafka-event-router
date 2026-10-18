// Package scripttest provides a Go-function-backed script.Runtime for engine tests and goldens.
package scripttest

import (
	"context"
	"sync"

	"github.com/eventrouter/router/pkg/engine/script"
)

// Func implements one fake script body.
type Func func(ctx context.Context, inv script.Invocation) (script.Result, error)

// BoundFunc builds a fake script body from its compiled source, so bodies can use what
// the route binds at compile time (ctx.lookup tables).
type BoundFunc func(src script.Source) Func

// FuncRuntime compiles code by exact-match lookup in a table of Go functions.
type FuncRuntime struct {
	mu    sync.RWMutex
	funcs map[string]BoundFunc
}

// NewFuncRuntime returns an empty runtime.
func NewFuncRuntime() *FuncRuntime {
	return &FuncRuntime{funcs: map[string]BoundFunc{}}
}

// Register binds code to fn.
func (r *FuncRuntime) Register(code string, fn Func) *FuncRuntime {
	return r.RegisterBound(code, func(script.Source) Func { return fn })
}

// RegisterBound binds code to a body built from the compiled source.
func (r *FuncRuntime) RegisterBound(code string, fn BoundFunc) *FuncRuntime {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.funcs[code] = fn
	return r
}

// Compile returns a module for registered code and a CompileError otherwise.
func (r *FuncRuntime) Compile(_ context.Context, src script.Source, _ script.Limits) (script.Module, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.funcs[src.Code]
	if !ok {
		return nil, &script.CompileError{Message: "SyntaxError: unknown test script", Line: 1, Column: 1}
	}
	return funcModule{fn: fn(src)}, nil
}

// funcModule adapts a Func to script.Module.
type funcModule struct {
	fn Func
}

func (m funcModule) Call(ctx context.Context, inv script.Invocation) (script.Result, error) {
	return m.fn(ctx, inv)
}

func (funcModule) Close(context.Context) error { return nil }
