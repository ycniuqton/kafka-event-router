package query

import (
	"net/http"
	"time"
)

// BackendConfig is what a querier factory needs to connect.
type BackendConfig struct {
	URL     string
	Timeout time.Duration
	Client  *http.Client
}

// Factory builds one backend from config.
type Factory func(cfg BackendConfig) (Querier, error)

// QuerierManager is the registry of metrics backends; the configured kind is selected at wiring.
type QuerierManager struct {
	factories map[BackendKind]Factory
}

// NewQuerierManager returns a manager with the built-in Prometheus and mock backends.
func NewQuerierManager() *QuerierManager {
	m := &QuerierManager{factories: map[BackendKind]Factory{}}
	m.Register(BackendPrometheus, func(cfg BackendConfig) (Querier, error) { return NewPrometheusQuerier(cfg) })
	m.Register(BackendMock, func(BackendConfig) (Querier, error) { return NewMockQuerier(time.Now), nil })
	return m
}

// Register adds or replaces a backend factory.
func (m *QuerierManager) Register(kind BackendKind, f Factory) {
	m.factories[kind] = f
}

// Select builds the backend of kind.
func (m *QuerierManager) Select(kind BackendKind, cfg BackendConfig) (Querier, error) {
	f, ok := m.factories[kind]
	if !ok {
		return nil, ErrUnknownBackend{Kind: kind}
	}
	return f(cfg)
}
