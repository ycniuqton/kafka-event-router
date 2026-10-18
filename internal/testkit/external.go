package testkit

import (
	"context"
	"fmt"
	"os"
)

// ExternalProvider hands out services that already run (CI service containers, the dev tunnel),
// addressed by the ROUTER_TEST_* environment variables.
type ExternalProvider struct {
	urls  map[Service]EnvKey
	token map[Service]EnvKey
}

// NewExternalProvider maps every service to its environment variable.
func NewExternalProvider() *ExternalProvider {
	return &ExternalProvider{
		urls: map[Service]EnvKey{
			ServicePostgres: EnvPostgresURL,
			ServiceRedis:    EnvRedisURL,
			ServiceKafka:    EnvKafka,
			ServiceVault:    EnvVaultAddr,

			ServiceSchemaRegistry: EnvSchemaRegistryURL,
		},
		token: map[Service]EnvKey{ServiceVault: EnvVaultToken},
	}
}

// Kind implements Provider.
func (p *ExternalProvider) Kind() ProviderKind {
	return ProviderExternal
}

// Provide returns the configured endpoint; nothing is started, so Release is a no-op.
func (p *ExternalProvider) Provide(_ context.Context, svc Service) (Endpoint, Release, error) {
	url := os.Getenv(string(p.urls[svc]))
	if url == "" {
		return Endpoint{}, nil, fmt.Errorf("%w: set %s", ErrNotConfigured, p.urls[svc])
	}
	return Endpoint{Service: svc, URL: url, Token: os.Getenv(string(p.token[svc]))}, func() {}, nil
}
