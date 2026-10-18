package enginebind

import (
	"context"
	"errors"

	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/sandbox"
)

// ScriptRuntime implements the engine's script.Runtime port for one route version.
type ScriptRuntime struct {
	Manager sandbox.Manager
	Scope   sandbox.RouteScope
}

// Compile loads one source into its own pooled module.
func (r ScriptRuntime) Compile(ctx context.Context, src script.Source, lim script.Limits) (script.Module, error) {
	srcs := []sandbox.Source{{StepPath: src.Origin, Entry: sandbox.EntryKind(src.Entry), Code: src.Code}}
	rs, err := r.Manager.Load(ctx, r.Scope, srcs, RouteLimitsOf(lim))
	if err != nil {
		return nil, ErrorMapper{}.Compile(err)
	}
	mod, _ := rs.Module(src.Origin)
	return scriptModule{mod: mod, entry: src.Entry, closer: rs.Close}, nil
}

// RouteLimitsOf converts engine limits (base units) to route-document units.
func RouteLimitsOf(lim script.Limits) sandbox.RouteLimits {
	return sandbox.RouteLimits{
		JSTimeoutMs:   lim.Timeout.Milliseconds(),
		JSMemoryMb:    lim.MemoryBytes / int64(sandbox.MiB),
		JSMaxOutputKb: lim.MaxOutputBytes / int64(sandbox.KiB),
	}
}

// scriptModule adapts a sandbox Module to script.Module; closer releases its sandbox.
type scriptModule struct {
	mod    sandbox.Module
	entry  script.EntryKind
	closer func(ctx context.Context) error
}

// Call converts the snapshot, runs the sandbox and converts the result back.
func (m scriptModule) Call(ctx context.Context, inv script.Invocation) (script.Result, error) {
	res, err := m.mod.Call(ctx, sandbox.Invocation{Event: SnapshotCodec{}.ToGuest(inv.Event), Now: inv.Now})
	out := script.Result{Logs: res.Logs}
	if err != nil {
		return out, ErrorMapper{}.Call(err)
	}
	out.Value = ResultValue{}.Of(m.entry, res)
	return out, nil
}

func (m scriptModule) Close(ctx context.Context) error { return m.closer(ctx) }

// ResultValue renders a sandbox Result as the engine expects: bool, nil, map or []any.
type ResultValue struct{}

func (ResultValue) Of(entry script.EntryKind, res sandbox.Result) any {
	if entry == script.EntryPredicate {
		return res.Bool
	}
	if res.Dropped {
		return nil
	}
	if len(res.Outputs) == 1 {
		return SnapshotCodec{}.FromGuest(res.Outputs[0])
	}
	list := make([]any, 0, len(res.Outputs))
	for _, o := range res.Outputs {
		list = append(list, SnapshotCodec{}.FromGuest(o))
	}
	return list
}

// ErrorMapper translates sandbox errors into the engine's script errors.
type ErrorMapper struct{}

// Compile maps compile-kind errors to *script.CompileError.
func (ErrorMapper) Compile(err error) error {
	se, ok := sandbox.AsError(err)
	if !ok || se.Kind != sandbox.ErrorCompile {
		return err
	}
	return &script.CompileError{Message: se.Message, Line: se.Line, Column: se.Column}
}

// Call keeps the sandbox error and marks timeouts with script.ErrTimeout.
func (ErrorMapper) Call(err error) error {
	if se, ok := sandbox.AsError(err); ok && se.Kind == sandbox.ErrorTimeout {
		return errors.Join(script.ErrTimeout, se)
	}
	return err
}
