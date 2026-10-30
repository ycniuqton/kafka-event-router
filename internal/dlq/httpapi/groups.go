package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/api"
)

// Workflow endpoint paths (design 00001 §10).
const (
	PathRouteGroups  Path = "/api/v1/routes/{id}/dlq/groups"
	PathGlobalGroups Path = "/api/v1/dlq/groups"
	PathReplay       Path = "/api/v1/routes/{id}/dlq/{dlqId}/replay"
	PathDryRun       Path = "/api/v1/routes/{id}/dlq/dry-run"
	PathJobsList     Path = "/api/v1/dlq/jobs"
	PathJobApprove   Path = "/api/v1/dlq/jobs/{jobId}/approve"
	PathJobReject    Path = "/api/v1/dlq/jobs/{jobId}/reject"
)

// GroupsHandler serves GET /routes/{id}/dlq/groups (cause groups of one route).
type GroupsHandler struct{ d Deps }

func (h *GroupsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathRouteGroups), Permission: PermRead, Object: RouteObject}
}

func (h *GroupsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	q, err := QueryParser{}.Parse(r)
	if err != nil {
		return err
	}
	page, err := h.d.Manager.Groups(r.Context(), chi.URLParam(r, string(ParamRouteID)), q)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, page)
}

// GlobalGroupsHandler serves GET /dlq/groups over every route the caller may read.
type GlobalGroupsHandler struct{ d Deps }

func (h *GlobalGroupsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathGlobalGroups)}
}

func (h *GlobalGroupsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	q, err := QueryParser{}.Parse(r)
	if err != nil {
		return err
	}
	all, err := h.d.Manager.Routes(r.Context())
	if err != nil {
		return err
	}
	readable := ReadableRoutes{Authorizer: h.d.Authorizer}.Filter(r.Context(), all, Values{}.Set(r.URL.Query().Get(string(QueryRouteID))))
	return h.d.Responder.JSON(w, http.StatusOK, h.d.Manager.GlobalGroups(r.Context(), readable, q))
}
