package sandboxinfo

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// SaltSource selects where a workload (worker) reads team hash salts from.
type SaltSource string

const (
	SaltSourceControlPlane SaltSource = "control_plane"
	SaltSourceProcess      SaltSource = "process"
)

// EnvSaltSource selects the workload salt source; control_plane keeps ctx.hash equal to preview and DLQ retry.
const EnvSaltSource config.EnvKey = "ROUTER_SANDBOX_SALTS"

// SaltSource returns the configured workload salt source (default control_plane).
func (l SettingsLoader) SaltSource() SaltSource {
	return SaltSource(l.str(EnvSaltSource, string(SaltSourceControlPlane)))
}

// SaltSourceRegistry maps salt sources to providers so workloads pick one by configuration.
type SaltSourceRegistry struct {
	providers map[SaltSource]host.SaltProvider
}

// NewSaltSourceRegistry registers the control-plane provider and the process-local stand-in.
func NewSaltSourceRegistry(controlPlane, process host.SaltProvider) *SaltSourceRegistry {
	return &SaltSourceRegistry{providers: map[SaltSource]host.SaltProvider{
		SaltSourceControlPlane: controlPlane,
		SaltSourceProcess:      process,
	}}
}

// Build returns the provider of source.
func (r *SaltSourceRegistry) Build(source SaltSource) (host.SaltProvider, error) {
	p, ok := r.providers[source]
	if !ok {
		return nil, fmt.Errorf("sandbox: no salt source registered for %q (%s)", source, EnvSaltSource)
	}
	return p, nil
}

// JSONFetcher reads a JSON document from the API internal listener (controlplane.Client).
type JSONFetcher interface {
	GetJSON(ctx context.Context, path string, out any) error
}

// ControlPlaneSalts reads team salts from GET /internal/v1/sandbox/teams/{teamId}/hash-salt; cached per team.
type ControlPlaneSalts struct {
	fetcher JSONFetcher
	mu      sync.Mutex
	cache   map[string][]byte
}

// NewControlPlaneSalts builds the provider over the workload's control-plane client.
func NewControlPlaneSalts(fetcher JSONFetcher) *ControlPlaneSalts {
	return &ControlPlaneSalts{fetcher: fetcher, cache: map[string][]byte{}}
}

// Salt implements host.SaltProvider and ctxhelpers.SaltProvider; failures are not cached.
func (s *ControlPlaneSalts) Salt(ctx context.Context, teamID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if salt, ok := s.cache[teamID]; ok {
		return salt, nil
	}
	path := strings.Replace(string(PathTeamSalt), string(ParamTeamPlaceholder), url.PathEscape(teamID), 1)
	var body TeamSaltResponse
	if err := s.fetcher.GetJSON(ctx, path, &body); err != nil {
		return nil, fmt.Errorf("sandbox: team salt: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(body.ValueB64)
	if err != nil || len(salt) == 0 {
		return nil, fmt.Errorf("sandbox: team salt: invalid value for team %q", teamID)
	}
	s.cache[teamID] = salt
	return salt, nil
}
