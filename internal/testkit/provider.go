package testkit

import (
	"context"
	"errors"
)

// ErrNotConfigured means the selected provider cannot supply the service here; tests skip on it.
var ErrNotConfigured = errors.New("testkit: service not configured for this provider")

// Release frees whatever a provider started for one endpoint.
type Release func()

// Provider supplies endpoints for backing services; implementations are selected by config.
type Provider interface {
	Kind() ProviderKind
	Provide(ctx context.Context, svc Service) (Endpoint, Release, error)
}
