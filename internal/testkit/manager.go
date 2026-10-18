package testkit

import (
	"context"
	"errors"
	"os"
	"testing"
)

// Manager is the harness entry point: it picks the Provider named by ROUTER_TEST_PROVIDER from its
// registry and turns "not configured here" into a test skip, so integration tests stay opt-in.
type Manager struct {
	providers map[ProviderKind]Provider
	selected  ProviderKind
}

// NewManager registers the external and container providers and reads the selection from env.
func NewManager() *Manager {
	m := &Manager{providers: map[ProviderKind]Provider{}, selected: DefaultProvider}
	m.Register(NewExternalProvider())
	m.Register(NewContainerProvider(DefaultContainerCatalog()))
	if kind := ProviderKind(os.Getenv(string(EnvProvider))); kind != "" {
		m.selected = kind
	}
	return m
}

// Register adds or replaces a provider implementation.
func (m *Manager) Register(p Provider) {
	m.providers[p.Kind()] = p
}

// Provider returns the selected provider, or nil when the configured kind is unknown.
func (m *Manager) Provider() Provider {
	return m.providers[m.selected]
}

// Require returns an endpoint for svc released at test cleanup; it skips when svc is not available.
func (m *Manager) Require(t testing.TB, svc Service) Endpoint {
	t.Helper()
	p := m.Provider()
	if p == nil {
		t.Fatalf("testkit: unknown %s=%q", EnvProvider, m.selected)
	}
	ep, release, err := p.Provide(context.Background(), svc)
	if errors.Is(err, ErrNotConfigured) {
		t.Skipf("testkit: %s via %s provider: %v", svc, p.Kind(), err)
	}
	if err != nil {
		t.Fatalf("testkit: %s: %v", svc, err)
	}
	t.Cleanup(release)
	return ep
}
