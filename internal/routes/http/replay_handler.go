package routeshttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/routes"
)

// Replay endpoints (P3): routes owns the API, route-runtime the offset reset, monitoring the dialog.
const (
	PathReplay       Path      = "/api/v1/routes/{id}/replay"
	PathReplayStatus Path      = "/api/v1/routes/{id}/replays/{replayId}"
	ParamReplayID    PathParam = "replayId"
)

// ReplayHandler serves POST /routes/{id}/replay → 202 with the pending replay.
type ReplayHandler struct{ d Deps }

func (h *ReplayHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathReplay), Permission: PermDeploy, Object: RouteObject,
		Mutating: true, AuditAction: routes.ActionRouteReplayed}
}

func (h *ReplayHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	body, err := api.Decoder[routes.ReplayRequest]{}.Decode(w, r)
	if err != nil {
		return err
	}
	rp, err := h.d.Replays.Request(r.Context(), id, body)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusAccepted, rp)
}

// ReplayStatusHandler serves GET /routes/{id}/replays/{replayId}.
type ReplayStatusHandler struct{ d Deps }

func (h *ReplayStatusHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathReplayStatus), Permission: PermRead, Object: RouteObject}
}

func (h *ReplayStatusHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	replay, err := Params{}.ReplayID(r)
	if err != nil {
		return err
	}
	rp, err := h.d.Replays.Status(r.Context(), id, replay)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, rp)
}

// ReplayID returns the {replayId} parameter; malformed ids are unknown replays (404).
func (Params) ReplayID(r *http.Request) (routes.ReplayID, error) {
	raw := chi.URLParam(r, string(ParamReplayID))
	if _, err := uuid.Parse(raw); err != nil {
		return "", routes.ErrReplayNotFound
	}
	return routes.ReplayID(raw), nil
}
