package connections

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/kafka"
)

// Clock returns the current time (replaced in tests).
type Clock func() time.Time

// ServiceDeps are the collaborators of the CRUD service.
type ServiceDeps struct {
	Repo      Repository
	Usage     UsageResolver
	Vault     *SecretVault
	Validator *Validator
	Tx        store.TxManager
	Clock     Clock
	Access    ConnectionAccess
	Secrets   SecretUsage
	Teardown  ClusterTeardown
}

// Service is the entry point for connection CRUD: validation, write-only secrets, usage guard, audit.
type Service struct {
	d       ServiceDeps
	auditor Auditor
	builder ConnectionBuilder
}

// NewService builds the service.
func NewService(d ServiceDeps) *Service {
	if d.Clock == nil {
		d.Clock = time.Now
	}
	if d.Teardown == nil {
		d.Teardown = NoTeardown{}
	}
	return &Service{d: d}
}

// List returns one page of connections and the used-by count of each.
func (s *Service) List(ctx context.Context, q ListQuery) (Page, map[uuid.UUID]int, error) {
	page, err := s.d.Repo.List(ctx, q)
	if err != nil {
		return Page{}, nil, err
	}
	ids := make([]uuid.UUID, 0, len(page.Items))
	for _, c := range page.Items {
		ids = append(ids, c.ID)
	}
	counts, err := s.d.Usage.CountsFor(ctx, ids)
	if err != nil {
		return Page{}, nil, err
	}
	return page, counts, nil
}

// Get returns one connection and the routes using it.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Connection, []RouteRef, error) {
	c, err := s.d.Repo.Get(ctx, id)
	if err != nil {
		return Connection{}, nil, err
	}
	routes, err := s.d.Usage.RoutesUsing(ctx, id)
	if err != nil {
		return Connection{}, nil, err
	}
	return c, routes, nil
}

// Create validates the draft, stores its secrets and the row in one transaction.
func (s *Service) Create(ctx context.Context, d Draft) (Connection, error) {
	d.Secrets = s.d.Secrets.Keep(d.Secrets, d.Spec())
	if err := s.d.Validator.Validate(ctx, d); err != nil {
		return Connection{}, err
	}
	var out Connection
	err := s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		ref, err := s.d.Vault.Store(ctx, d.Secrets)
		if err != nil {
			return err
		}
		c := s.builder.New(d, ref)
		if out, err = s.d.Repo.Create(ctx, c); err != nil {
			return err
		}
		return s.auditor.Record(ctx, ActionCreated, out, nil, ViewOf(out))
	})
	return out, err
}

// Update applies a merged draft under If-Match; present secrets replace stored ones.
func (s *Service) Update(ctx context.Context, id uuid.UUID, expected store.RowVersion, d Draft) (Connection, error) {
	var out Connection
	err := s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		current, err := s.d.Repo.Get(ctx, id)
		if err != nil {
			return err
		}
		if err := (api.Precondition{}).Compare(expected, current.Version); err != nil {
			return err
		}
		next, err := s.prepareUpdate(ctx, current, d)
		if err != nil {
			return err
		}
		if out, err = s.d.Repo.Update(ctx, next, expected); err != nil {
			return err
		}
		return s.auditUpdate(ctx, current, out, d.Secrets)
	})
	return out, err
}

// prepareUpdate validates the draft against the stored secrets, drops unused ones and rotates the bundle.
func (s *Service) prepareUpdate(ctx context.Context, current Connection, d Draft) (Connection, error) {
	stored, err := s.d.Vault.Load(ctx, current.SecretRef)
	if err != nil {
		return Connection{}, err
	}
	check := d
	check.Secrets = s.d.Secrets.Keep(stored.Merge(d.Secrets), d.Spec())
	if err := s.d.Validator.Validate(ctx, check); err != nil {
		return Connection{}, err
	}
	ref := current.SecretRef
	if check.Secrets != stored {
		if ref, err = s.d.Vault.Rewrite(ctx, current.SecretRef, check.Secrets); err != nil {
			return Connection{}, err
		}
	}
	next := s.builder.Apply(current, d, ref, SecretFieldsOf(check.Secrets).List())
	if !d.Secrets.IsEmpty() {
		next.Health = Health{Status: HealthUnknown}
	}
	return next, nil
}

func (s *Service) auditUpdate(ctx context.Context, before, after Connection, patch kafka.SecretBundle) error {
	if err := s.auditor.Record(ctx, ActionUpdated, after, ViewOf(before), ViewOf(after)); err != nil {
		return err
	}
	if patch.IsEmpty() {
		return nil
	}
	return s.auditor.Record(ctx, ActionSecretReplaced, after, nil, SecretAudit{Replaced: SecretFieldsOf(patch).List()})
}

// Delete removes an unused connection and its secret, then releases its runtime resources;
// used connections yield InUseError.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	routes, err := s.d.Usage.RoutesUsing(ctx, id)
	if err != nil {
		return err
	}
	if len(routes) > 0 {
		return &InUseError{Routes: routes}
	}
	var deleted Connection
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		current, err := s.d.Repo.Get(ctx, id)
		if err != nil {
			return err
		}
		if err := s.d.Repo.Delete(ctx, id); err != nil {
			return err
		}
		if err := s.d.Vault.Drop(ctx, current.SecretRef); err != nil {
			return err
		}
		deleted = current
		return s.auditor.Record(ctx, ActionDeleted, current, ViewOf(current), nil)
	})
	if err == nil {
		s.d.Teardown.Release(ctx, deleted)
	}
	return err
}

// ConnectionBuilder maps drafts onto connection entities.
type ConnectionBuilder struct{}

// New builds a fresh, untested connection.
func (ConnectionBuilder) New(d Draft, ref secrets.Ref) Connection {
	return Connection{
		TeamID: d.TeamID, Name: d.Name, Description: d.Description, EnvTag: d.EnvTag, BootstrapServers: d.BootstrapServers,
		Security: d.Security, Properties: d.Properties, SchemaRegistry: d.SchemaRegistry, SecretRef: ref, SecretFields: SecretFieldsOf(d.Secrets).List(),
		Sensitive: d.Sensitive, AllowTopicCreate: d.AllowTopicCreate, ProbeEnabled: d.ProbeEnabled, Kind: d.KindOrDefault(),
		Health: Health{Status: HealthUnknown},
	}
}

// Apply overlays a draft onto current; brokers or security changes reset health to unknown.
func (b ConnectionBuilder) Apply(current Connection, d Draft, ref secrets.Ref, fields []kafka.SecretField) Connection {
	next := b.New(d, ref)
	next.ID, next.TeamID, next.TeamName, next.Kind = current.ID, current.TeamID, current.TeamName, current.Kind
	next.SecretFields, next.Version, next.CreatedAt = fields, current.Version, current.CreatedAt
	next.Health = current.Health
	if next.TransportChanged(current) {
		next.Health = Health{Status: HealthUnknown}
	}
	return next
}
