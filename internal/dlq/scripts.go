package dlq

import (
	"context"
	"fmt"

	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/engines"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// retryParallelism is the sandbox parallelism of a retry (records run one by one).
type retryParallelism int

const retrySandboxParallelism retryParallelism = 1

// SandboxScriptRuntimes runs js steps of retried events in the QuickJS sandbox (worker scope).
type SandboxScriptRuntimes struct {
	Manager sandbox.Manager
}

// NewSandboxScriptRuntimes starts a sandbox manager with the given team salts.
func NewSandboxScriptRuntimes(ctx context.Context, salts host.SaltProvider) (*SandboxScriptRuntimes, error) {
	m, err := sandbox.NewManager(ctx, sandbox.DefaultConfig(), sandbox.ManagerDeps{
		Engines: engines.Registry(), Salts: salts, Binder: enginebind.Binder{Base: ctxhelpers.Set{}},
	})
	if err != nil {
		return nil, fmt.Errorf("dlq: sandbox: %w", err)
	}
	return &SandboxScriptRuntimes{Manager: m}, nil
}

// Runtime implements ScriptRuntimes.
func (s *SandboxScriptRuntimes) Runtime(route RouteInfo, v VersionConfig) script.Runtime {
	return enginebind.ScriptRuntime{Manager: s.Manager, Scope: sandbox.RouteScope{
		RouteID: route.ID, VersionID: v.ID, VersionNo: v.No, TeamID: route.TeamID, Parallelism: int(retrySandboxParallelism), Mode: sandbox.ScopeWorker,
	}}
}
