package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/api"
)

// SummaryHandler serves GET /routes/{id}/dlq/summary.
type SummaryHandler struct{ d Deps }

func (h *SummaryHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathRouteSummary), Permission: PermRead, Object: RouteObject}
}

func (h *SummaryHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	s, err := h.d.Manager.Summary(r.Context(), chi.URLParam(r, string(ParamRouteID)))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, s)
}

// GlobalSummaryHandler serves GET /dlq/summary (nav badge).
type GlobalSummaryHandler struct{ d Deps }

func (h *GlobalSummaryHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathGlobalSum)}
}

func (h *GlobalSummaryHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	all, err := h.d.Manager.Routes(r.Context())
	if err != nil {
		return err
	}
	readable := ReadableRoutes{Authorizer: h.d.Authorizer}.Filter(r.Context(), all, nil)
	return h.d.Responder.JSON(w, http.StatusOK, h.d.Manager.GlobalSummary(r.Context(), readable))
}
