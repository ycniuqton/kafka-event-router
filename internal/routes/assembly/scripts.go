package assembly

import (
	"context"
	"errors"
	"fmt"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/engines"
)

// ValidationScope names the pseudo route the API compiles user code under.
type ValidationScope string

const validationRouteID ValidationScope = "validation"

// ErrNotExecutable is returned if a validation-only module is ever called.
var ErrNotExecutable = errors.New("routes: validation-only script modules cannot run")

// ScriptChecker provides the script runtime used by the compile dry-run of validation.
type ScriptChecker interface {
	Kind() ScriptCheckKind
	Runtime() script.Runtime
	Close(ctx context.Context) error
}

// SandboxScriptChecker compiles JS in the QuickJS sandbox, so syntax and entry errors surface as issues.
type SandboxScriptChecker struct {
	Manager sandbox.Manager
}

// NewSandboxScriptChecker starts a sandbox manager with a fixed salt (validation never executes events).
func NewSandboxScriptChecker(ctx context.Context) (*SandboxScriptChecker, error) {
	m, err := sandbox.NewManager(ctx, sandbox.DefaultConfig(), sandbox.ManagerDeps{
		Engines: engines.Registry(), Salts: ValidationSalts{}, Binder: enginebind.Binder{Base: ctxhelpers.Set{}},
	})
	if err != nil {
		return nil, fmt.Errorf("routes: sandbox: %w", err)
	}
	return &SandboxScriptChecker{Manager: m}, nil
}

func (*SandboxScriptChecker) Kind() ScriptCheckKind { return ScriptCheckSandbox }

func (c *SandboxScriptChecker) Runtime() script.Runtime {
	return enginebind.ScriptRuntime{Manager: c.Manager, Scope: sandbox.RouteScope{RouteID: string(validationRouteID), Parallelism: 1, Mode: sandbox.ScopeTest}}
}

func (c *SandboxScriptChecker) Close(ctx context.Context) error { return c.Manager.Close(ctx) }

// ValidationSalts returns a constant salt: validation compiles code but never hashes event data.
type ValidationSalts struct{}

// ValidationSalt is the fixed salt of validation compiles.
type ValidationSalt string

const validationSalt ValidationSalt = "routes-validation"

func (ValidationSalts) Salt(context.Context, string) ([]byte, error) {
	return []byte(validationSalt), nil
}

// SkipScriptChecker accepts every script without compiling it (JS errors then surface at preview / rollout).
type SkipScriptChecker struct{}

func (SkipScriptChecker) Kind() ScriptCheckKind       { return ScriptCheckSkip }
func (SkipScriptChecker) Runtime() script.Runtime     { return acceptingRuntime{} }
func (SkipScriptChecker) Close(context.Context) error { return nil }

// acceptingRuntime compiles any source into a module that refuses to run.
type acceptingRuntime struct{}

func (acceptingRuntime) Compile(context.Context, script.Source, script.Limits) (script.Module, error) {
	return refusingModule{}, nil
}

// refusingModule is a compiled placeholder.
type refusingModule struct{}

func (refusingModule) Call(context.Context, script.Invocation) (script.Result, error) {
	return script.Result{}, ErrNotExecutable
}

func (refusingModule) Close(context.Context) error { return nil }

// ScriptCheckerFactory builds a checker.
type ScriptCheckerFactory func(ctx context.Context) (ScriptChecker, error)

// ScriptCheckers maps kinds to factories (selected by ROUTER_ROUTES_SCRIPT_CHECK).
var ScriptCheckers = map[ScriptCheckKind]ScriptCheckerFactory{
	ScriptCheckSandbox: func(ctx context.Context) (ScriptChecker, error) { return NewSandboxScriptChecker(ctx) },
	ScriptCheckSkip:    func(context.Context) (ScriptChecker, error) { return SkipScriptChecker{}, nil },
}
