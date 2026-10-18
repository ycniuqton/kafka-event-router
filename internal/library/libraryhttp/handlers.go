package libraryhttp

import (
	"net/http"
	"strings"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/library"
	"github.com/eventrouter/router/internal/platform/api"
)

// ListSeparator splits the comma-separated names filter.
type ListSeparator string

const NamesSeparator ListSeparator = ","

// ListHandler serves GET /api/v1/{transforms|lookup-tables}?q=&names=&archived=&cursor=&limit=.
type ListHandler struct {
	d Deps
	k KindRoutes
}

// Route implements api.Handler.
func (h *ListHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(h.k.Collection), Permission: PermRead}
}

// Serve implements api.Handler.
func (h *ListHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	page, err := h.d.Pages.Parse(r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	query := library.ListQuery{Text: q.Get(string(QueryText)), Limit: page.Limit, After: page.After,
		IncludeArchived: q.Get(string(QueryArchived)) == "true", Names: ListNames{}.Of(q[string(QueryNames)])}
	res, err := h.d.Service.List(r.Context(), h.k.Kind, query)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, api.PageResponse[ItemDTO]{Items: Presenter{}.Items(res.Items), NextCursor: h.d.Pages.Next(res.Next)})
}

// ListNames parses repeated and comma-separated names= values.
type ListNames struct{}

// Of returns the non-empty names (nil when none).
func (ListNames) Of(values []string) []string {
	var out []string
	for _, v := range values {
		for _, n := range strings.Split(v, string(NamesSeparator)) {
			if n = strings.TrimSpace(n); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

// CreateHandler serves POST /api/v1/{transforms|lookup-tables} {name, teamId?, description, keyType?, steps|rows, changeNote}.
type CreateHandler struct {
	d Deps
	k KindRoutes
}

// Route implements api.Handler.
func (h *CreateHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(h.k.Collection), Permission: PermWrite, Mutating: true,
		AuditAction: h.k.Created, BodyLimit: api.BodyLimitLarge}
}

// Serve implements api.Handler.
func (h *CreateHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	in, err := api.Decoder[ObjectInput]{Limit: api.BodyLimitLarge}.Decode(w, r)
	if err != nil {
		return err
	}
	p := auth.PrincipalFrom(r.Context())
	team, err := OwnerTeamPolicy{}.Resolve(p, in.TeamID)
	if err != nil {
		return err
	}
	it, err := h.d.Service.Create(r.Context(), h.k.Kind, library.Draft{TeamID: team, Name: in.Name, Description: in.Description,
		KeyType: in.KeyType, Content: in.Content(h.k.Content), ChangeNote: in.ChangeNote, Author: p.UserID})
	if err != nil {
		return err
	}
	if err := (Auditor{}).Record(r, h.k.Created, it); err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusCreated, it.RowVersion, Presenter{}.Item(it))
}

// GetHandler serves GET /api/v1/{kind}/{id} (ETag = row version).
type GetHandler struct {
	d Deps
	k KindRoutes
}

// Route implements api.Handler.
func (h *GetHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(h.k.Item), Permission: PermRead}
}

// Serve implements api.Handler.
func (h *GetHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := ObjectID{}.Of(r)
	if err != nil {
		return err
	}
	it, err := h.d.Service.Get(r.Context(), h.k.Kind, id)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, it.RowVersion, Presenter{}.Item(it))
}

// PatchHandler serves PATCH /api/v1/{kind}/{id} {description} with If-Match.
type PatchHandler struct {
	d Deps
	k KindRoutes
}

// Route implements api.Handler.
func (h *PatchHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPatch, Pattern: string(h.k.Item), Permission: PermWrite, Object: h.d.OwnedObject(h.k),
		Mutating: true, AuditAction: h.k.Updated, Versioned: true}
}

// Serve implements api.Handler.
func (h *PatchHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := ObjectID{}.Of(r)
	if err != nil {
		return err
	}
	expected, err := api.Precondition{}.Require(r)
	if err != nil {
		return err
	}
	in, err := api.Decoder[PatchInput]{}.Decode(w, r)
	if err != nil {
		return err
	}
	it, err := h.d.Service.UpdateMetadata(r.Context(), h.k.Kind, id, library.MetadataPatch{Description: in.Description}, expected)
	if err != nil {
		return err
	}
	if err := (Auditor{}).Record(r, h.k.Updated, it); err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, it.RowVersion, Presenter{}.Item(it))
}

// ArchiveHandler serves DELETE /api/v1/{kind}/{id}: archive, 409 while routes use the object.
type ArchiveHandler struct {
	d Deps
	k KindRoutes
}

// Route implements api.Handler.
func (h *ArchiveHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodDelete, Pattern: string(h.k.Item), Permission: PermWrite, Object: h.d.OwnedObject(h.k),
		Mutating: true, AuditAction: h.k.Archived}
}

// Serve implements api.Handler.
func (h *ArchiveHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := ObjectID{}.Of(r)
	if err != nil {
		return err
	}
	it, err := h.d.Service.Archive(r.Context(), h.k.Kind, id)
	if err != nil {
		return err
	}
	if err := (Auditor{}).Record(r, h.k.Archived, it); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// Auditor records library writes in the request's audit scope (name, version and metadata, never the content).
type Auditor struct{}

// Record writes one entry; requests without a scope (tests) record nothing.
func (Auditor) Record(r *http.Request, action audit.Action, it library.Item) error {
	scope, ok := audit.FromContext(r.Context())
	if !ok {
		return nil
	}
	team := it.TeamID
	return scope.Record(r.Context(), audit.Change{Action: action, Object: audit.ObjectType(it.Kind), ObjectID: it.ID.String(), TeamID: &team,
		After: ItemAudit{Name: it.Name, Version: it.LatestVersion, Size: it.Size, Description: it.Description, Archived: it.ArchivedAt != nil}})
}
