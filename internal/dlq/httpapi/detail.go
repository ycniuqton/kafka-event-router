package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
)

// DetailHandler serves GET /routes/{id}/dlq/{dlqId}.
type DetailHandler struct{ d Deps }

func (h *DetailHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathRouteItem), Permission: PermRead, Object: RouteObject}
}

// Serve renders the detail inside its own audit scope (GET): every view is audited with its masked flag.
func (h *DetailHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	ctx := audit.WithScope(r.Context(), audit.NewScope(api.AuditContext(r), h.d.Recorder, nil))
	v, err := h.d.Manager.Get(ctx, Caller{}.Viewer(r), chi.URLParam(r, string(ParamRouteID)), chi.URLParam(r, string(ParamDLQID)))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, v)
}

// StudioHandler serves POST /routes/{id}/dlq/{dlqId}/studio.
type StudioHandler struct{ d Deps }

func (h *StudioHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathStudio), Permission: PermStudio, Object: RouteObject}
}

func (h *StudioHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	link, err := h.d.Manager.OpenInStudio(r.Context(), Caller{}.Viewer(r), chi.URLParam(r, string(ParamRouteID)), chi.URLParam(r, string(ParamDLQID)))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, link)
}
