package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
)

// Create validates, compiles and stores a new object with its first version (409 on a taken name).
func (s *Service) Create(ctx context.Context, kind ObjectKind, d Draft) (Item, error) {
	k, err := s.d.Kinds.Get(kind)
	if err != nil {
		return Item{}, err
	}
	if err := (DraftRules{Limits: s.d.Limits}).Check(k, d); err != nil {
		return Item{}, err
	}
	if err := s.nameFree(ctx, k, d.Name); err != nil {
		return Item{}, err
	}
	rec, err := s.record(ctx, k, d.TeamID, VersionDraft{Content: d.Content, ChangeNote: d.ChangeNote, Author: d.Author})
	if err != nil {
		return Item{}, err
	}
	var id uuid.UUID
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if id, err = s.d.Repo.Create(ctx, k, d); err != nil {
			return err
		}
		_, err = s.d.Repo.AppendVersion(ctx, k, id, rec)
		return err
	})
	if err != nil {
		return Item{}, err
	}
	s.d.Metrics.Published(kind)
	return s.d.Repo.Get(ctx, k, id)
}

// Publish appends an immutable version (latest+1) to an existing, non-archived object.
func (s *Service) Publish(ctx context.Context, kind ObjectKind, id uuid.UUID, v VersionDraft) (Item, error) {
	k, err := s.d.Kinds.Get(kind)
	if err != nil {
		return Item{}, err
	}
	it, err := s.d.Repo.Get(ctx, k, id)
	if err != nil {
		return Item{}, err
	}
	if it.ArchivedAt != nil {
		return Item{}, problem.NewError(problem.KindConflict, string(MsgArchived))
	}
	if _, err := k.Validate(v.Content, it.KeyType); err != nil {
		return Item{}, err
	}
	if err := (DraftRules{Limits: s.d.Limits}).note(v.ChangeNote); err != nil {
		return Item{}, err
	}
	rec, err := s.record(ctx, k, it.TeamID, v)
	if err != nil {
		return Item{}, err
	}
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		_, err := s.d.Repo.AppendVersion(ctx, k, id, rec)
		return err
	})
	if err != nil {
		return Item{}, err
	}
	s.d.Metrics.Published(kind)
	return s.d.Repo.Get(ctx, k, id)
}

// Import publishes a lookup table version parsed from a CSV or JSON body (422 with /lines/{n}, 413 past limits).
func (s *Service) Import(ctx context.Context, id uuid.UUID, contentType string, body io.Reader, v VersionDraft) (Item, error) {
	it, err := s.Get(ctx, KindLookupTable, id)
	if err != nil {
		return Item{}, err
	}
	rows, err := s.d.Importers.Import(contentType, body, it.KeyType)
	if err != nil {
		return Item{}, err
	}
	if v.Content, err = json.Marshal(rows); err != nil {
		return Item{}, err
	}
	return s.Publish(ctx, KindLookupTable, id, v)
}

// UpdateMetadata changes the description (If-Match on the object's row version).
func (s *Service) UpdateMetadata(ctx context.Context, kind ObjectKind, id uuid.UUID, p MetadataPatch, expected store.RowVersion) (Item, error) {
	k, err := s.d.Kinds.Get(kind)
	if err != nil {
		return Item{}, err
	}
	if p.Description != nil {
		if err := (DraftRules{Limits: s.d.Limits}).description(*p.Description); err != nil {
			return Item{}, err
		}
	}
	if err := s.d.Repo.UpdateMetadata(ctx, k, id, p, expected); err != nil {
		return Item{}, err
	}
	return s.d.Repo.Get(ctx, k, id)
}

// Archive hides an object from listings; refused (409) while any route references it.
func (s *Service) Archive(ctx context.Context, kind ObjectKind, id uuid.UUID) (Item, error) {
	usages, err := s.Usages(ctx, kind, id)
	if err != nil {
		return Item{}, err
	}
	if len(usages) > 0 {
		return Item{}, InUseError{Usages: usages}
	}
	k, _ := s.d.Kinds.Get(kind)
	if err := s.d.Repo.Archive(ctx, k, id); err != nil {
		return Item{}, err
	}
	return s.d.Repo.Get(ctx, k, id)
}

// record compiles the content for the owner team and fingerprints it.
func (s *Service) record(ctx context.Context, k ContentKind, team uuid.UUID, v VersionDraft) (VersionRecord, error) {
	if err := k.Compile(ctx, v.Content, team); err != nil {
		return VersionRecord{}, err
	}
	hash, err := ContentHasher{}.Hash(v.Content)
	if err != nil {
		return VersionRecord{}, err
	}
	return VersionRecord{Content: v.Content, ConfigHash: hash, ChangeNote: v.ChangeNote, Author: v.Author}, nil
}

// nameFree refuses a name already used by an object of the kind in any team (refs are global).
func (s *Service) nameFree(ctx context.Context, k ContentKind, name string) error {
	_, err := s.d.Repo.GetByName(ctx, k, name)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return problem.NewError(problem.KindConflict, string(MsgNameTaken)).At(string(FieldName))
}

// DraftRules validates a new object's name, metadata and first version (422 with JSON Pointer paths).
type DraftRules struct {
	Limits Limits
}

// Check returns every issue of d.
func (r DraftRules) Check(k ContentKind, d Draft) error {
	verr := api.NewValidationError()
	if d.Name == "" {
		verr.Add(string(FieldName), string(CodeRequired), string(MsgNameRequired))
	} else if !NamePattern.MatchString(d.Name) {
		verr.Add(string(FieldName), string(CodeInvalid), string(MsgNamePattern))
	}
	if k.Schema().KeyType != "" && !d.KeyType.OrDefault().Known() {
		verr.Add(string(FieldKeyType), string(CodeInvalid), string(MsgKeyType))
	}
	for _, err := range []error{r.description(d.Description), r.note(d.ChangeNote)} {
		r.merge(verr, err)
	}
	_, err := k.Validate(d.Content, d.KeyType)
	if !r.merge(verr, err) {
		return err
	}
	return verr.OrNil()
}

func (r DraftRules) description(text string) error {
	if len(text) > r.Limits.MaxDescription {
		return api.NewValidationError().Add(string(FieldDescription), string(CodeTooLarge), string(MsgDescription))
	}
	return nil
}

func (r DraftRules) note(text string) error {
	if len(text) > r.Limits.MaxChangeNote {
		return api.NewValidationError().Add(string(FieldChangeNote), string(CodeTooLarge), string(MsgChangeNote))
	}
	return nil
}

// merge appends err's field errors to verr; false when err is not a validation error.
func (DraftRules) merge(verr *api.ValidationError, err error) bool {
	if err == nil {
		return true
	}
	var ve *api.ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	verr.Errors = append(verr.Errors, ve.Errors...)
	return true
}

// InUseError refuses archiving an object that routes still reference (409 with the route names).
type InUseError struct {
	Usages []Usage
}

func (e InUseError) Error() string {
	names := make([]string, 0, len(e.Usages))
	for _, u := range e.Usages {
		names = append(names, u.RouteName)
	}
	return fmt.Sprintf(string(MsgInUse), len(e.Usages), strings.Join(names, ", "))
}

// ProblemKind implements problem.Classified.
func (InUseError) ProblemKind() problem.Kind { return problem.KindConflict }

// ProblemCode implements problem.Coded.
func (InUseError) ProblemCode() string { return string(CodeInUse) }

// RowsDecoder turns paged row elements into a RowsPage with the next offset cursor.
type RowsDecoder struct{}

// Page decodes elems.
func (RowsDecoder) Page(elems []json.RawMessage, total int, q RowsQuery) (RowsPage, error) {
	page := RowsPage{Rows: make([]LookupRow, 0, len(elems)), Total: total}
	for _, e := range elems {
		var row LookupRow
		if err := json.Unmarshal(e, &row); err != nil {
			return RowsPage{}, err
		}
		page.Rows = append(page.Rows, row)
	}
	if next := q.Offset + len(elems); next < total && len(elems) > 0 {
		page.Next = &store.Cursor{ID: fmt.Sprint(next)}
	}
	return page, nil
}
