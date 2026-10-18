package preview

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/kafka"
)

// RegistryCachePolicy bounds how long a connection's registry settings are reused before re-reading them.
type RegistryCachePolicy time.Duration

const RegistryConfigTTL RegistryCachePolicy = RegistryCachePolicy(30 * time.Second)

// registryAuthModes maps connection registry auth modes onto the codec client's modes.
var registryAuthModes = map[kafka.RegistryAuthMode]codec.RegistryAuthMode{
	kafka.RegistryAuthNone:   codec.RegistryAuthNone,
	kafka.RegistryAuthBasic:  codec.RegistryAuthBasic,
	kafka.RegistryAuthBearer: codec.RegistryAuthBearer,
}

// SpecRegistryConfigs reads registry settings from connection specs (connections#CN-102), caching them
// briefly so decoding a page or a run does not re-read the connection per record.
type SpecRegistryConfigs struct {
	Specs ConnectionSpecSource
	Now   func() time.Time
	mu    sync.Mutex
	cache map[string]registryConfigEntry
}

type registryConfigEntry struct {
	cfg     codec.RegistryConfig
	ok      bool
	expires time.Time
}

// RegistryConfig implements codec.RegistryConfigSource; ok is false for connections without a registry.
func (s *SpecRegistryConfigs) RegistryConfig(ctx context.Context, connectionID string) (codec.RegistryConfig, bool, error) {
	if e, hit := s.cached(connectionID); hit {
		return e.cfg, e.ok, nil
	}
	spec, err := s.Specs.Spec(ctx, connectionID)
	if err != nil {
		return codec.RegistryConfig{}, false, err
	}
	entry := registryConfigEntry{expires: s.Now().Add(time.Duration(RegistryConfigTTL))}
	if spec.SchemaRegistry != nil && spec.SchemaRegistry.URL != "" {
		entry.cfg, entry.ok = RegistryConfigMapper{}.Of(spec), true
	}
	s.store(connectionID, entry)
	return entry.cfg, entry.ok, nil
}

func (s *SpecRegistryConfigs) cached(id string) (registryConfigEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cache[id]
	return e, ok && s.Now().Before(e.expires)
}

func (s *SpecRegistryConfigs) store(id string, e registryConfigEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache == nil {
		s.cache = map[string]registryConfigEntry{}
	}
	s.cache[id] = e
}

// RegistryConfigMapper turns a connection spec's registry block into the codec client config.
type RegistryConfigMapper struct{}

// Of maps spec (which must have a registry); the secret is the password (basic) or token (bearer).
func (RegistryConfigMapper) Of(spec kafka.ConnectionSpec) codec.RegistryConfig {
	reg := spec.SchemaRegistry
	mode := registryAuthModes[reg.Auth.ModeOrDefault()]
	cfg := codec.RegistryConfig{URL: reg.URL, Auth: mode, Username: reg.Auth.Username}
	secret := spec.Secret.RegistrySecret
	if mode == codec.RegistryAuthBearer {
		cfg.Token = secret
	} else {
		cfg.Password = secret
	}
	return cfg
}

// NoRegistries is the provider of ROUTER_PREVIEW_SCHEMA_REGISTRY=none: no connection has a registry.
type NoRegistries struct{}

// Registry implements codec.RegistryProvider.
func (NoRegistries) Registry(context.Context, string) (codec.SchemaRegistry, error) { return nil, nil }

// RegistryProviderFactory builds a registry provider.
type RegistryProviderFactory func() codec.RegistryProvider

// RegistryProviderRegistry selects the configured provider (ROUTER_PREVIEW_SCHEMA_REGISTRY).
type RegistryProviderRegistry struct {
	factories map[SchemaRegistryKind]RegistryProviderFactory
}

// NewRegistryProviderRegistry registers control_plane (spec-backed HTTP clients) and none.
func NewRegistryProviderRegistry(specs ConnectionSpecSource, now func() time.Time) *RegistryProviderRegistry {
	return &RegistryProviderRegistry{factories: map[SchemaRegistryKind]RegistryProviderFactory{
		SchemaRegistryControlPlane: func() codec.RegistryProvider {
			return &codec.HTTPRegistryProvider{Source: &SpecRegistryConfigs{Specs: specs, Now: now}}
		},
		SchemaRegistryNone: func() codec.RegistryProvider { return NoRegistries{} },
	}}
}

// Build returns the provider of kind.
func (r *RegistryProviderRegistry) Build(kind SchemaRegistryKind) (codec.RegistryProvider, error) {
	f, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf("preview: no schema registry provider registered for %q", kind)
	}
	return f(), nil
}
