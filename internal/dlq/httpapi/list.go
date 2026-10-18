package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/api"
)

// ListHandler serves GET /routes/{id}/dlq.
type ListHandler struct{ d Deps }

func (h *ListHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathRouteList), Permission: PermRead, Object: RouteObject}
}

func (h *ListHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	q, err := QueryParser{}.Parse(r)
	if err != nil {
		return err
	}
	page, err := h.d.Manager.List(r.Context(), chi.URLParam(r, string(ParamRouteID)), q)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, page)
}

// GlobalListHandler serves GET /dlq: every route the caller may read, merged newest first.
type GlobalListHandler struct{ d Deps }

func (h *GlobalListHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathGlobalList)}
}

func (h *GlobalListHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	q, err := QueryParser{}.Parse(r)
	if err != nil {
		return err
	}
	all, err := h.d.Manager.Routes(r.Context())
	if err != nil {
		return err
	}
	readable := ReadableRoutes{Authorizer: h.d.Authorizer}.Filter(r.Context(), all, Values{}.Set(r.URL.Query().Get(string(QueryRouteID))))
	page, err := h.d.Manager.ListGlobal(r.Context(), readable, q)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, page)
}
