package secrets

import (
	"context"
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/problem"
)

// ProviderKind names a master-key provider implementation, selected by config.
type ProviderKind = config.SecretsProviderKind

const (
	ProviderLocal = config.SecretsLocal
	ProviderVault = config.SecretsVault
	ProviderKMS   = config.SecretsKMS
)

// RefPrefix marks secret references (`sec_<ulid>`).
type RefPrefix string

const SecretRefPrefix RefPrefix = "sec_"

// Ref is the opaque reference stored in connections.secret_ref.
type Ref string

// NewRef generates a fresh reference.
func NewRef() Ref {
	return Ref(string(SecretRefPrefix) + strings.ToLower(ulid.Make().String()))
}

// Valid reports whether r has the reference format.
func (r Ref) Valid() bool {
	return strings.HasPrefix(string(r), string(SecretRefPrefix)) && len(r) > len(SecretRefPrefix)
}

// Provider wraps and unwraps per-secret data keys with a master key.
type Provider interface {
	Kind() ProviderKind
	KeyID() string
	WrapKey(ctx context.Context, dek []byte) ([]byte, error)
	UnwrapKey(ctx context.Context, wrapped []byte, keyID string) ([]byte, error)
}

// Manager is the only entry point business logic uses for secret storage.
type Manager interface {
	Put(ctx context.Context, plaintext []byte) (Ref, error)
	Get(ctx context.Context, ref Ref) ([]byte, error)
	Replace(ctx context.Context, ref Ref, plaintext []byte) error
	Delete(ctx context.Context, ref Ref) error
}

var (
	// ErrUnavailable is returned when the master-key provider cannot be reached.
	ErrUnavailable = problem.NewError(problem.KindUnavailable, "Secret store unreachable. Try again; nothing was saved.")
	// ErrNotFound is returned for unknown references.
	ErrNotFound = problem.NewError(problem.KindNotFound, "The secret does not exist.")
)

// ProviderFactory constructs a Provider from configuration; implementations register one per kind.
type ProviderFactory func(ctx context.Context, cfg config.SecretsConfig) (Provider, error)

// Registry selects the configured Provider without switching on concrete types.
type Registry struct {
	factories map[ProviderKind]ProviderFactory
}

// NewRegistry returns an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{factories: map[ProviderKind]ProviderFactory{}}
}

// Register adds a factory for a provider kind.
func (r *Registry) Register(kind ProviderKind, f ProviderFactory) {
	r.factories[kind] = f
}

// Build instantiates the provider named in configuration.
func (r *Registry) Build(ctx context.Context, cfg config.SecretsConfig) (Provider, error) {
	f, ok := r.factories[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("secrets: no provider registered for %q", cfg.Provider)
	}
	return f(ctx, cfg)
}
