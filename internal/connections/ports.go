package connections

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/kafka"
)

// Repository persists connections (implemented by pgstore.ConnectionRepository).
type Repository interface {
	Get(ctx context.Context, id uuid.UUID) (Connection, error)
	List(ctx context.Context, q ListQuery) (Page, error)
	Create(ctx context.Context, c Connection) (Connection, error)
	Update(ctx context.Context, c Connection, expected store.RowVersion) (Connection, error)
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateHealth(ctx context.Context, id uuid.UUID, h Health) error
	ListForProbe(ctx context.Context, checkedBefore time.Time, limit int) ([]Connection, error)
}

// UsageResolver finds the routes (draft or active version) that use a connection.
type UsageResolver interface {
	RoutesUsing(ctx context.Context, id uuid.UUID) ([]RouteRef, error)
	CountsFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error)
}

// ClientOpener opens cluster clients (implemented by kafka.ClientFactoryRegistry).
type ClientOpener interface {
	NewClient(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose) (kafka.Client, error)
}

// SpecResolver turns a stored connection into a client spec with its decrypted secrets.
type SpecResolver struct {
	repo  Repository
	vault *SecretVault
}

// NewSpecResolver builds the resolver.
func NewSpecResolver(repo Repository, vault *SecretVault) *SpecResolver {
	return &SpecResolver{repo: repo, vault: vault}
}

// Resolve loads the row and its secret bundle.
func (r *SpecResolver) Resolve(ctx context.Context, id uuid.UUID) (Connection, kafka.ConnectionSpec, error) {
	c, err := r.repo.Get(ctx, id)
	if err != nil {
		return Connection{}, kafka.ConnectionSpec{}, err
	}
	bundle, err := r.vault.Load(ctx, c.SecretRef)
	if err != nil {
		return Connection{}, kafka.ConnectionSpec{}, err
	}
	return c, c.Spec(bundle), nil
}
