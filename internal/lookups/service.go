package lookups

import (
	"context"
	"errors"
	"reflect"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/lookup"
)

// Repository persists sources (implemented by pgstore.Repository).
type Repository interface {
	Get(ctx context.Context, id uuid.UUID) (Source, error)
	List(ctx context.Context, q ListQuery) (Page, error)
	Create(ctx context.Context, s Source) (Source, error)
	Update(ctx context.Context, s Source, expected store.RowVersion) (Source, error)
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateHealth(ctx context.Context, id uuid.UUID, h Health) error
}

// UsageResolver finds the routes (draft or active version) whose lookup steps use a source.
type UsageResolver interface {
	RoutesUsing(ctx context.Context, id uuid.UUID) ([]RouteRef, error)
	CountsFor(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error)
}

// ServiceDeps are the collaborators of the service.
type ServiceDeps struct {
	Repo      Repository
	Usage     UsageResolver
	Vault     *SecretVault
	Validator Validator
	Tx        store.TxManager
}

// Service is the entry point for lookup-source CRUD, spec resolution and ownership checks.
type Service struct {
	d       ServiceDeps
	auditor Auditor
}

// NewService builds the service.
func NewService(d ServiceDeps) *Service {
	if d.Validator.Kinds == nil {
		d.Validator.Kinds = DefaultKinds()
	}
	return &Service{d: d}
}

// List returns one page and the used-by count of each source.
func (s *Service) List(ctx context.Context, q ListQuery) (Page, map[uuid.UUID]int, error) {
	page, err := s.d.Repo.List(ctx, q)
	if err != nil {
		return Page{}, nil, err
	}
	ids := make([]uuid.UUID, 0, len(page.Items))
	for _, src := range page.Items {
		ids = append(ids, src.ID)
	}
	counts, err := s.d.Usage.CountsFor(ctx, ids)
	return page, counts, err
}

// Get returns one source and the routes using it.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Source, []RouteRef, error) {
	src, err := s.d.Repo.Get(ctx, id)
	if err != nil {
		return Source{}, nil, err
	}
	routes, err := s.d.Usage.RoutesUsing(ctx, id)
	return src, routes, err
}

// Create validates the draft, stores its secrets and the row in one transaction.
func (s *Service) Create(ctx context.Context, d Draft) (Source, error) {
	d = s.d.Validator.Prepare(d)
	if err := s.d.Validator.Validate(d); err != nil {
		return Source{}, err
	}
	var out Source
	err := s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		ref, err := s.d.Vault.Store(ctx, d.Secrets)
		if err != nil {
			return err
		}
		if out, err = s.d.Repo.Create(ctx, SourceBuilder{}.New(d, ref)); err != nil {
			return err
		}
		return s.auditor.Record(ctx, ActionCreated, out, nil, ViewOf(out))
	})
	return out, err
}

// Update applies a merged draft under If-Match; present secrets replace stored ones.
func (s *Service) Update(ctx context.Context, id uuid.UUID, expected store.RowVersion, d Draft) (Source, error) {
	var out Source
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

// prepareUpdate validates the draft with the merged secrets and rewrites the stored bundle when it changed.
func (s *Service) prepareUpdate(ctx context.Context, current Source, d Draft) (Source, error) {
	stored, err := s.d.Vault.Load(ctx, current.SecretRef)
	if err != nil {
		return Source{}, s.secretError(err)
	}
	d.Kind = current.Kind
	check := d
	check.Secrets = stored.Merge(d.Secrets)
	check = s.d.Validator.Prepare(check)
	if err := s.d.Validator.Validate(check); err != nil {
		return Source{}, err
	}
	ref := current.SecretRef
	if check.Secrets != stored {
		if ref, err = s.d.Vault.Rewrite(ctx, current.SecretRef, check.Secrets); err != nil {
			return Source{}, err
		}
	}
	return SourceBuilder{}.Apply(current, check, ref, !d.Secrets.IsEmpty()), nil
}

func (s *Service) auditUpdate(ctx context.Context, before, after Source, patch lookup.SecretBundle) error {
	if err := s.auditor.Record(ctx, ActionUpdated, after, ViewOf(before), ViewOf(after)); err != nil {
		return err
	}
	if patch.IsEmpty() {
		return nil
	}
	return s.auditor.Record(ctx, ActionSecretReplaced, after, nil, SecretAudit{Replaced: patch.Set()})
}

// Delete removes an unused source and its secret; used sources yield InUseError.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	routes, err := s.d.Usage.RoutesUsing(ctx, id)
	if err != nil {
		return err
	}
	if len(routes) > 0 {
		return &InUseError{Routes: routes}
	}
	return s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
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
		return s.auditor.Record(ctx, ActionDeleted, current, ViewOf(current), nil)
	})
}

// Resolve returns the source and its runtime spec with decrypted secrets (internal spec endpoint, tests).
func (s *Service) Resolve(ctx context.Context, id uuid.UUID) (Source, lookup.SourceSpec, error) {
	src, err := s.d.Repo.Get(ctx, id)
	if err != nil {
		return Source{}, lookup.SourceSpec{}, err
	}
	bundle, err := s.d.Vault.Load(ctx, src.SecretRef)
	if err != nil {
		return Source{}, lookup.SourceSpec{}, s.secretError(err)
	}
	return src, src.Spec(bundle), nil
}

// Spec implements lookup.SpecProvider for in-process gateways (API side).
func (s *Service) Spec(ctx context.Context, id string) (lookup.SourceSpec, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return lookup.SourceSpec{}, lookup.ErrSourceNotFound
	}
	_, spec, err := s.Resolve(ctx, parsed)
	if errors.Is(err, store.ErrNotFound) {
		return lookup.SourceSpec{}, lookup.ErrSourceNotFound
	}
	return spec, err
}

// TeamOf returns the owner team of a source (rbac.TeamOwner for team-scoped permission checks).
func (s *Service) TeamOf(ctx context.Context, id string) (string, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return "", store.ErrNotFound
	}
	src, err := s.d.Repo.Get(ctx, parsed)
	if err != nil {
		return "", err
	}
	return src.TeamID.String(), nil
}

// RecordHealth stores a test result without bumping row_version.
func (s *Service) RecordHealth(ctx context.Context, id uuid.UUID, h Health) error {
	return s.d.Repo.UpdateHealth(ctx, id, h)
}

func (s *Service) secretError(err error) error {
	if errors.Is(err, secrets.ErrUnavailable) {
		return err
	}
	return ErrSecretUnavailable.Wrap(err)
}

// SourceBuilder maps drafts onto sources.
type SourceBuilder struct{}

// New builds a fresh, untested source.
func (SourceBuilder) New(d Draft, ref secrets.Ref) Source {
	return Source{TeamID: d.TeamID, Name: d.Name, Description: d.Description, Kind: d.Kind, EnvTag: d.EnvTag, Sensitive: d.Sensitive,
		Settings: d.Settings, Limits: d.Limits, SecretRef: ref, SecretFields: d.Secrets.Set(), Health: Health{Status: HealthUnknown}}
}

// Apply overlays a validated draft onto current; settings or secret changes reset health to unknown.
func (b SourceBuilder) Apply(current Source, d Draft, ref secrets.Ref, secretsChanged bool) Source {
	next := b.New(d, ref)
	next.ID, next.TeamID, next.TeamName, next.Kind = current.ID, current.TeamID, current.TeamName, current.Kind
	next.Version, next.CreatedAt, next.Health = current.Version, current.CreatedAt, current.Health
	if secretsChanged || !(SettingsCompare{}).Equal(current.Settings, d.Settings) {
		next.Health = Health{Status: HealthUnknown}
	}
	return next
}

// Health statuses written by this feature (shared with connections).
const (
	HealthUnknown     = connections.HealthUnknown
	HealthHealthy     = connections.HealthHealthy
	HealthUnreachable = connections.HealthUnreachable
	HealthAuthFailed  = connections.HealthAuthFailed
	HealthDegraded    = connections.HealthDegraded
)

// SettingsCompare compares kind settings.
type SettingsCompare struct{}

// Equal reports whether two settings are identical.
func (SettingsCompare) Equal(a, b lookup.Settings) bool { return reflect.DeepEqual(a, b) }
