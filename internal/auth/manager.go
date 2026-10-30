package auth

import (
	"fmt"
	"net/http"

	"github.com/eventrouter/router/internal/platform/config"
)

// Manager runs the configured authenticators in order; the first match wins.
type Manager struct {
	chain []Authenticator
}

// NewManager returns a Manager over the given chain.
func NewManager(chain ...Authenticator) *Manager {
	return &Manager{chain: chain}
}

// Authenticate returns the principal of r, or ErrUnauthenticated when nothing matched.
func (m *Manager) Authenticate(r *http.Request) (Principal, error) {
	for _, a := range m.chain {
		p, outcome, err := a.Authenticate(r)
		if err != nil {
			return Anonymous, err
		}
		if outcome == OutcomeMatched {
			return p, nil
		}
		if outcome == OutcomeRejected {
			return Anonymous, ErrUnauthenticated
		}
	}
	return Anonymous, ErrUnauthenticated
}

// AuthenticatorFactory builds an authenticator implementation.
type AuthenticatorFactory func() (Authenticator, error)

// Registry maps auth modes to authenticator kinds and kinds to factories (config-selected).
type Registry struct {
	modeKinds map[AuthMode]AuthenticatorKind
	factories map[AuthenticatorKind]AuthenticatorFactory
}

// NewRegistry returns a registry where oidc, dev and password logins share the session-cookie authenticator.
func NewRegistry() *Registry {
	return &Registry{
		modeKinds: map[AuthMode]AuthenticatorKind{
			config.AuthModeOIDC:     AuthenticatorSession,
			config.AuthModeDev:      AuthenticatorSession,
			config.AuthModeAPIToken: AuthenticatorAPIToken,
			config.AuthModePassword: AuthenticatorSession,
		},
		factories: map[AuthenticatorKind]AuthenticatorFactory{},
	}
}

// Register adds the factory of an authenticator kind.
func (r *Registry) Register(kind AuthenticatorKind, f AuthenticatorFactory) {
	r.factories[kind] = f
}

// Build instantiates one authenticator per distinct kind needed by modes, in mode order.
func (r *Registry) Build(modes []AuthMode) (*Manager, error) {
	seen := map[AuthenticatorKind]bool{}
	var chain []Authenticator
	for _, mode := range modes {
		kind, ok := r.modeKinds[mode]
		if !ok {
			return nil, fmt.Errorf("auth: unknown auth mode %q", mode)
		}
		if seen[kind] {
			continue
		}
		a, err := r.build(kind)
		if err != nil {
			return nil, err
		}
		seen[kind] = true
		chain = append(chain, a)
	}
	return NewManager(chain...), nil
}

func (r *Registry) build(kind AuthenticatorKind) (Authenticator, error) {
	f, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf("auth: no authenticator registered for %q", kind)
	}
	return f()
}
