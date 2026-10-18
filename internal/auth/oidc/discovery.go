package oidc

import (
	"context"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
)

// CheckName is the readiness check name of the IdP discovery.
type CheckName string

const CheckOIDC CheckName = "oidc"

// RefreshInterval is how long a discovered provider document is reused.
type RefreshInterval time.Duration

const DefaultRefreshInterval RefreshInterval = RefreshInterval(time.Hour)

// Discovery lazily fetches and caches the IdP's discovery document so the API starts even when the IdP is down.
type Discovery struct {
	issuer    string
	mu        sync.Mutex
	provider  *gooidc.Provider
	fetchedAt time.Time
	now       func() time.Time
}

// NewDiscovery builds a lazy discovery for issuer.
func NewDiscovery(issuer string, now func() time.Time) *Discovery {
	return &Discovery{issuer: issuer, now: now}
}

// Provider returns the cached provider, refreshing it once per interval; stale data is kept on failure.
func (d *Discovery) Provider(ctx context.Context) (*gooidc.Provider, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.provider != nil && d.now().Sub(d.fetchedAt) < time.Duration(DefaultRefreshInterval) {
		return d.provider, nil
	}
	p, err := gooidc.NewProvider(ctx, d.issuer)
	if err != nil && d.provider != nil {
		return d.provider, nil
	}
	if err != nil {
		return nil, err
	}
	d.provider, d.fetchedAt = p, d.now()
	return p, nil
}

// Check implements httpserver.ReadinessCheck (used only when oidc is the sole mode).
func (d *Discovery) Check(ctx context.Context) error {
	_, err := d.Provider(ctx)
	return err
}

// Name implements httpserver.ReadinessCheck.
func (d *Discovery) Name() string {
	return string(CheckOIDC)
}
