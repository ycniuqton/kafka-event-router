package library

import (
	"context"
	"regexp"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// NamePattern is the allowed shape of library object names (they are referenced by name from routes).
var NamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ServiceDeps are the collaborators of the library service.
type ServiceDeps struct {
	Kinds     *KindManager
	Repo      *Repository
	Usages    *UsageRepository
	Importers *RowImporterManager
	Tx        store.TxManager
	Limits    Limits
	Metrics   PublishMetrics
}

// Service is the library Manager business code talks to: list, read, create, publish, archive, usages.
type Service struct {
	d ServiceDeps
}

// NewService builds the service.
func NewService(d ServiceDeps) *Service {
	if d.Importers == nil {
		d.Importers = NewRowImporterManager(d.Limits)
	}
	if d.Metrics == nil {
		d.Metrics = NoPublishMetrics{}
	}
	return &Service{d: d}
}

// List returns one page of objects of a kind (open read: every team's objects).
func (s *Service) List(ctx context.Context, kind ObjectKind, q ListQuery) (ListPage, error) {
	k, err := s.d.Kinds.Get(kind)
	if err != nil {
		return ListPage{}, err
	}
	if q.Limit <= 0 || q.Limit > s.d.Limits.MaxListItems {
		q.Limit = s.d.Limits.MaxListItems
	}
	if len(q.Names) > s.d.Limits.MaxNameCandidates {
		q.Names = q.Names[:s.d.Limits.MaxNameCandidates]
	}
	return s.d.Repo.List(ctx, k, q)
}

// Get returns one object.
func (s *Service) Get(ctx context.Context, kind ObjectKind, id uuid.UUID) (Item, error) {
	k, err := s.d.Kinds.Get(kind)
	if err != nil {
		return Item{}, err
	}
	return s.d.Repo.Get(ctx, k, id)
}

// Versions returns every version of an object, newest first.
func (s *Service) Versions(ctx context.Context, kind ObjectKind, id uuid.UUID) ([]Version, error) {
	k, err := s.d.Kinds.Get(kind)
	if err != nil {
		return nil, err
	}
	if _, err := s.d.Repo.Get(ctx, k, id); err != nil {
		return nil, err
	}
	return s.d.Repo.Versions(ctx, k, id)
}

// Version returns one version of an object.
func (s *Service) Version(ctx context.Context, kind ObjectKind, id uuid.UUID, no int) (Version, error) {
	k, err := s.d.Kinds.Get(kind)
	if err != nil {
		return Version{}, err
	}
	return s.d.Repo.Version(ctx, k, id, no)
}

// Usages returns the routes referencing the object (draft or active version) with their pins.
func (s *Service) Usages(ctx context.Context, kind ObjectKind, id uuid.UUID) ([]Usage, error) {
	k, err := s.d.Kinds.Get(kind)
	if err != nil {
		return nil, err
	}
	it, err := s.d.Repo.Get(ctx, k, id)
	if err != nil {
		return nil, err
	}
	return s.d.Usages.Usages(ctx, k, it.Name)
}

// Rows pages the rows of one version of a lookup table.
func (s *Service) Rows(ctx context.Context, id uuid.UUID, q RowsQuery) (RowsPage, error) {
	k, err := s.d.Kinds.Get(KindLookupTable)
	if err != nil {
		return RowsPage{}, err
	}
	if _, err := s.d.Repo.Version(ctx, k, id, q.Version); err != nil {
		return RowsPage{}, err
	}
	elems, total, err := s.d.Repo.Elements(ctx, k, id, q)
	if err != nil {
		return RowsPage{}, err
	}
	return RowsDecoder{}.Page(elems, total, q)
}

// Owner returns a TeamOwner for object-scoped permission checks of one kind.
func (s *Service) Owner(kind ObjectKind) KindOwner {
	return KindOwner{service: s, kind: kind}
}

// KindOwner resolves the owning team of objects of one kind (rbac.TeamOwner).
type KindOwner struct {
	service *Service
	kind    ObjectKind
}

// TeamOf implements rbac.TeamOwner.
func (o KindOwner) TeamOf(ctx context.Context, id string) (string, error) {
	k, err := o.service.d.Kinds.Get(o.kind)
	if err != nil {
		return "", err
	}
	return o.service.d.Repo.TeamOf(ctx, k, id)
}
