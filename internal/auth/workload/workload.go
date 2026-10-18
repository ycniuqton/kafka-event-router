// Package workload authenticates worker/preview processes on the internal listener
// (static token locally, Kubernetes TokenReview in clusters, mTLS in P2).
package workload

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/problem"
)

// Kind is the configured workload authentication kind.
type Kind = config.WorkloadAuthKind

// Scheme names the Authorization header scheme workloads use.
type Scheme string

const SchemeBearer Scheme = "Bearer "

// HeaderName names request headers read by workload authentication.
type HeaderName string

const HeaderAuthorization HeaderName = "Authorization"

// ErrUnauthenticated rejects unknown or invalid workload credentials (no secret data is logged).
var ErrUnauthenticated = problem.NewError(problem.KindUnauthenticated, "Workload credential missing or invalid.")

// Authenticator is the base of every workload authenticator.
type Authenticator interface {
	Kind() Kind
	Authenticate(r *http.Request) (auth.Principal, error)
}

// Factory builds a workload authenticator from configuration.
type Factory func(cfg config.InternalConfig) (Authenticator, error)

// Registry selects the configured workload authenticator.
type Registry struct {
	factories map[Kind]Factory
}

// NewRegistry returns a registry with every kind (static_token, k8s_token, mtls).
func NewRegistry() *Registry {
	r := &Registry{factories: map[Kind]Factory{}}
	r.Register(config.WorkloadStaticToken, func(cfg config.InternalConfig) (Authenticator, error) {
		return NewStaticTokenAuthenticator(cfg.WorkloadToken)
	})
	r.Register(config.WorkloadK8sToken, func(cfg config.InternalConfig) (Authenticator, error) {
		return NewK8sTokenReviewAuthenticator(cfg.K8s)
	})
	r.Register(config.WorkloadMTLS, func(cfg config.InternalConfig) (Authenticator, error) {
		return NewMTLSAuthenticator(cfg.MTLSAllowed), nil
	})
	return r
}

// Register adds a factory.
func (r *Registry) Register(kind Kind, f Factory) {
	r.factories[kind] = f
}

// Build instantiates the configured authenticator.
func (r *Registry) Build(cfg config.InternalConfig) (*Manager, error) {
	f, ok := r.factories[cfg.WorkloadAuth]
	if !ok {
		return nil, fmt.Errorf("workload: no authenticator registered for %q", cfg.WorkloadAuth)
	}
	a, err := f(cfg)
	if err != nil {
		return nil, err
	}
	return &Manager{authenticator: a}, nil
}

// Manager is what the internal listener talks to; it implements api.PrincipalAuthenticator.
type Manager struct {
	authenticator Authenticator
}

// NewManager wraps an authenticator (useful in tests).
func NewManager(a Authenticator) *Manager {
	return &Manager{authenticator: a}
}

// Authenticate returns a workload principal or ErrUnauthenticated.
func (m *Manager) Authenticate(r *http.Request) (auth.Principal, error) {
	return m.authenticator.Authenticate(r)
}

// BearerToken extracts the bearer token of r, or "".
func BearerToken(r *http.Request) string {
	h := r.Header.Get(string(HeaderAuthorization))
	if !strings.HasPrefix(h, string(SchemeBearer)) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, string(SchemeBearer)))
}
