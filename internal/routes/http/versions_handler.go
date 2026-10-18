package routeshttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/routes"
)

// VersionsHandler serves GET /routes/{id}/versions.
type VersionsHandler struct{ d Deps }

func (h *VersionsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathVersions), Permission: PermRead, Object: RouteObject}
}

func (h *VersionsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	page, err := Params{}.Page(h.d, r)
	if err != nil {
		return err
	}
	items, next, err := h.d.Versions.List(r.Context(), id, page)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, api.PageResponse[routes.VersionSummary]{Items: items, NextCursor: h.d.Pages.Next(next)})
}

// VersionHandler serves GET /routes/{id}/versions/{no}.
type VersionHandler struct{ d Deps }

func (h *VersionHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathVersion), Permission: PermRead, Object: RouteObject}
}

func (h *VersionHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	no, err := Params{}.VersionNo(r)
	if err != nil {
		return err
	}
	v, err := h.d.Versions.Get(r.Context(), id, no)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, v)
}

// DiffHandler serves GET /routes/{id}/versions/{a}/diff/{b}.
type DiffHandler struct{ d Deps }

func (h *DiffHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathDiff), Permission: PermRead, Object: RouteObject}
}

func (h *DiffHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	d, err := h.d.Versions.Diff(r.Context(), id, chi.URLParam(r, string(ParamA)), chi.URLParam(r, string(ParamB)))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, d)
}
