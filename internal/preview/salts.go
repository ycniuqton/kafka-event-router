package preview

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"sync"

	"github.com/eventrouter/router/pkg/sandbox/host"
)

// SaltsKind selects where preview reads team hash salts.
type SaltsKind string

const (
	SaltsProcess      SaltsKind = "process"
	SaltsControlPlane SaltsKind = "control_plane"
)

// PathTeamSalt is the API internal path of a team's hash salt (js-sandbox, migration 0006).
const PathTeamSalt InternalPath = "/internal/v1/sandbox/teams/%s/hash-salt"

// SaltProviderRegistry selects the configured salt provider (ROUTER_PREVIEW_SALTS).
type SaltProviderRegistry struct {
	providers map[SaltsKind]host.SaltProvider
}

// NewSaltProviderRegistry registers the process-local and control-plane providers.
func NewSaltProviderRegistry(controlPlane host.SaltProvider) *SaltProviderRegistry {
	return &SaltProviderRegistry{providers: map[SaltsKind]host.SaltProvider{
		SaltsProcess:      NewProcessSaltProvider(),
		SaltsControlPlane: controlPlane,
	}}
}

// Build returns the provider of kind.
func (r *SaltProviderRegistry) Build(kind SaltsKind) (host.SaltProvider, error) {
	p, ok := r.providers[kind]
	if !ok {
		return nil, fmt.Errorf("preview: no salt provider registered for %q", kind)
	}
	return p, nil
}

// ControlPlaneSalts reads team salts from the API so ctx.hash in preview equals the worker's; cached per team.
type ControlPlaneSalts struct {
	fetcher JSONFetcher
	mu      sync.Mutex
	cache   map[string][]byte
}

// NewControlPlaneSalts builds the provider over the control-plane client.
func NewControlPlaneSalts(fetcher JSONFetcher) *ControlPlaneSalts {
	return &ControlPlaneSalts{fetcher: fetcher, cache: map[string][]byte{}}
}

// Salt implements host.SaltProvider.
func (s *ControlPlaneSalts) Salt(ctx context.Context, teamID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if salt, ok := s.cache[teamID]; ok {
		return salt, nil
	}
	var body struct {
		ValueB64 string `json:"value_b64"`
	}
	if err := s.fetcher.GetJSON(ctx, fmt.Sprintf(string(PathTeamSalt), url.PathEscape(teamID)), &body); err != nil {
		return nil, fmt.Errorf("preview: team salt: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(body.ValueB64)
	if err != nil {
		return nil, fmt.Errorf("preview: team salt: %w", err)
	}
	s.cache[teamID] = salt
	return salt, nil
}

// UnreachableSalts fails every lookup with the error that kept the control-plane client from building.
type UnreachableSalts struct {
	Err error
}

// Salt implements host.SaltProvider.
func (s UnreachableSalts) Salt(context.Context, string) ([]byte, error) {
	return nil, fmt.Errorf("preview: team salt: %w", s.Err)
}
