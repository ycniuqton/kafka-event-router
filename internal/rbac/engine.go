package rbac

import (
	"context"
	"fmt"

	"github.com/eventrouter/router/internal/platform/config"
)

// EngineKind selects a PolicyEngine implementation (ROUTER_RBAC_ENGINE).
type EngineKind = config.RBACEngineKind

// PolicyEngine is the base of the extensible policy domain (built-in role bindings now, OPA in P3).
type PolicyEngine interface {
	Kind() EngineKind
	Evaluate(ctx context.Context, s Subject, p Permission, r Resource) Decision
	Permissions(s Subject, team TeamID) PermissionSet
}

// EngineDeps are the inputs every engine factory receives.
type EngineDeps struct {
	Matrix  RoleMatrix
	Catalog PermissionCatalog
}

// EngineFactory builds one engine implementation.
type EngineFactory func(EngineDeps) (PolicyEngine, error)

// EngineRegistry maps engine kinds to factories (config-selected, no switch on concrete types).
type EngineRegistry struct {
	factories map[EngineKind]EngineFactory
}

// NewEngineRegistry returns a registry with the built-in role-binding engine registered.
func NewEngineRegistry() *EngineRegistry {
	reg := &EngineRegistry{factories: map[EngineKind]EngineFactory{}}
	reg.Register(config.RBACEngineRoleBinding, func(d EngineDeps) (PolicyEngine, error) {
		return NewRoleBindingEngine(d.Matrix, d.Catalog), nil
	})
	return reg
}

// Register adds or replaces the factory of kind.
func (r *EngineRegistry) Register(kind EngineKind, f EngineFactory) {
	r.factories[kind] = f
}

// Build instantiates the configured engine.
func (r *EngineRegistry) Build(kind EngineKind, deps EngineDeps) (PolicyEngine, error) {
	f, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf("rbac: no policy engine registered for %q", kind)
	}
	return f(deps)
}
