package routeshttp

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

// CreateBody is the body of POST /routes.
type CreateBody struct {
	Name        string          `json:"name"`
	TeamID      string          `json:"teamId"`
	Description string          `json:"description"`
	Tags        []string        `json:"tags"`
	Draft       json.RawMessage `json:"draft"`
}

// PatchBody is the body of PATCH /routes/{id}; absent members are left unchanged.
type PatchBody struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Tags        *[]string       `json:"tags"`
	Replicas    *int            `json:"replicas"`
	Draft       json.RawMessage `json:"draft"`
}

// DuplicateBody is the optional body of POST /routes/{id}/duplicate.
type DuplicateBody struct {
	Name string `json:"name"`
}

// Writer renders route detail responses with their ETag.
type Writer struct {
	d Deps
}

// Detail writes a RouteDetail with status and ETag.
func (wr Writer) Detail(w http.ResponseWriter, status int, d routes.RouteDetail) error {
	return wr.d.Responder.Versioned(w, status, store.RowVersion(d.Revision), d)
}

// Created writes 201 with Location.
func (wr Writer) Created(w http.ResponseWriter, d routes.RouteDetail) error {
	w.Header().Set(string(api.HeaderLocation), strings.Replace(string(PathItem), "{"+string(ParamID)+"}", string(d.ID), 1))
	return wr.Detail(w, http.StatusCreated, d)
}

// ListHandler serves GET /routes.
type ListHandler struct{ d Deps }

func (h *ListHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathCollection), Permission: PermRead}
}

func (h *ListHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	q, err := ListQueryParser{}.Parse(r)
	if err != nil {
		return err
	}
	page, err := Params{}.Page(h.d, r)
	if err != nil {
		return err
	}
	items, next, err := h.d.Routes.List(r.Context(), q, page)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, api.PageResponse[routes.RouteSummary]{Items: items, NextCursor: h.d.Pages.Next(next)})
}

// CreateHandler serves POST /routes.
type CreateHandler struct{ d Deps }

func (h *CreateHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathCollection), Permission: PermWrite,
		Mutating: true, AuditAction: routes.ActionRouteCreated, BodyLimit: api.BodyLimitLarge}
}

func (h *CreateHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	body, err := api.Decoder[CreateBody]{Limit: api.BodyLimitLarge}.Decode(w, r)
	if err != nil {
		return err
	}
	d, err := h.d.Routes.Create(r.Context(), routes.NewRoute{Name: body.Name, TeamID: routes.TeamID(body.TeamID),
		Description: body.Description, Tags: body.Tags, Draft: body.Draft})
	if err != nil {
		return err
	}
	return Writer{d: h.d}.Created(w, d)
}

// GetHandler serves GET /routes/{id}.
type GetHandler struct{ d Deps }

func (h *GetHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathItem), Permission: PermRead, Object: RouteObject}
}

func (h *GetHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	d, err := h.d.Routes.Get(r.Context(), id)
	if err != nil {
		return err
	}
	return Writer{d: h.d}.Detail(w, http.StatusOK, d)
}

// PatchHandler serves PATCH /routes/{id} (settings and draft, If-Match required).
type PatchHandler struct{ d Deps }

func (h *PatchHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPatch, Pattern: string(PathItem), Permission: PermWrite, Object: RouteObject,
		Mutating: true, AuditAction: routes.ActionRouteDraftSaved, Versioned: true, BodyLimit: api.BodyLimitLarge}
}

func (h *PatchHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	expected, err := api.Precondition{}.Require(r)
	if err != nil {
		return err
	}
	body, err := api.Decoder[PatchBody]{Limit: api.BodyLimitLarge}.Decode(w, r)
	if err != nil {
		return err
	}
	change := routes.RouteChange{Name: body.Name, Description: body.Description, Tags: body.Tags, Replicas: body.Replicas, Draft: body.Draft}
	d, err := h.d.Routes.Update(r.Context(), id, change, routes.Revision(expected))
	if err != nil {
		return err
	}
	return Writer{d: h.d}.Detail(w, http.StatusOK, d)
}

// ArchiveHandler serves DELETE /routes/{id} (archive; idempotent).
type ArchiveHandler struct{ d Deps }

func (h *ArchiveHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodDelete, Pattern: string(PathItem), Permission: PermWrite, Object: RouteObject,
		Mutating: true, AuditAction: routes.ActionRouteArchived}
}

func (h *ArchiveHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	if err := h.d.Routes.Archive(r.Context(), id); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// DuplicateHandler serves POST /routes/{id}/duplicate.
type DuplicateHandler struct{ d Deps }

func (h *DuplicateHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathDuplicate), Permission: PermWrite, Object: RouteObject,
		Mutating: true, AuditAction: routes.ActionRouteDuplicated}
}

func (h *DuplicateHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	body, err := api.Decoder[DuplicateBody]{AllowEmpty: true}.Decode(w, r)
	if err != nil {
		return err
	}
	d, err := h.d.Routes.Duplicate(r.Context(), id, body.Name)
	if err != nil {
		return err
	}
	return Writer{d: h.d}.Created(w, d)
}
