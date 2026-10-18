// Package engines builds the EngineRegistry with every compiled-in sandbox engine; bootstraps
// pass it to sandbox.NewManager so the engine is selected by ROUTER_SANDBOX_ENGINE.
package engines

import (
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/quickjs"
)

// Registry returns a registry with all built-in engines.
func Registry() *sandbox.EngineRegistry {
	reg := sandbox.NewEngineRegistry()
	quickjs.Register(reg)
	return reg
}
