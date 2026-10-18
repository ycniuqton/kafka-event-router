package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/rbac"
)

// TeamList is the body of GET /teams.
type TeamList struct {
	Items []rbac.Team `json:"items"`
}

// ListTeamsHandler serves GET /teams (any member).
type ListTeamsHandler struct{ d Deps }

func (h *ListTeamsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathTeams), Permission: rbac.PermTeamRead.Auth()}
}

func (h *ListTeamsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	teams, err := h.d.Teams.List(r.Context())
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, TeamList{Items: teams})
}

// GetTeamHandler serves GET /teams/{id} with an ETag.
type GetTeamHandler struct{ d Deps }

func (h *GetTeamHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathTeam), Permission: rbac.PermTeamRead.Auth()}
}

func (h *GetTeamHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := UUIDReader{}.Path(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return err
	}
	t, err := h.d.Teams.Get(r.Context(), id)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, t.RowVersion, t)
}

// TeamRequest is the body of POST /teams and PATCH /teams/{id}.
type TeamRequest struct {
	Name                *string `json:"name"`
	RequireProdApproval *bool   `json:"requireProdApproval"`
}

// CreateTeamHandler serves POST /teams (workspace admins).
type CreateTeamHandler struct{ d Deps }

func (h *CreateTeamHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathTeams), Permission: rbac.PermTeamManage.Auth(),
		Mutating: true, AuditAction: audit.ActionTeamCreated}
}

func (h *CreateTeamHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	req, err := api.Decoder[TeamRequest]{}.Decode(w, r)
	if err != nil {
		return err
	}
	name := ""
	if req.Name != nil {
		name = *req.Name
	}
	t, err := h.d.Teams.Create(r.Context(), rbac.CreateTeam{Name: name, RequireProdApproval: req.RequireProdApproval})
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusCreated, t.RowVersion, t)
}

// PatchTeamHandler serves PATCH /teams/{id} (If-Match required).
type PatchTeamHandler struct{ d Deps }

func (h *PatchTeamHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPatch, Pattern: string(PathTeam), Permission: rbac.PermTeamManage.Auth(),
		Mutating: true, AuditAction: audit.ActionTeamUpdated, Versioned: true}
}

func (h *PatchTeamHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := UUIDReader{}.Path(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return err
	}
	version, err := api.Precondition{}.Require(r)
	if err != nil {
		return err
	}
	req, err := api.Decoder[TeamRequest]{}.Decode(w, r)
	if err != nil {
		return err
	}
	t, err := h.d.Teams.Update(r.Context(), id, rbac.TeamChange{Name: req.Name, RequireProdApproval: req.RequireProdApproval}, version)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, t.RowVersion, t)
}

// DeleteTeamHandler serves DELETE /teams/{id} (If-Match required; 409 team_not_empty).
type DeleteTeamHandler struct{ d Deps }

func (h *DeleteTeamHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodDelete, Pattern: string(PathTeam), Permission: rbac.PermTeamManage.Auth(),
		Mutating: true, AuditAction: audit.ActionTeamDeleted, Versioned: true}
}

func (h *DeleteTeamHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := UUIDReader{}.Path(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return err
	}
	version, err := api.Precondition{}.Require(r)
	if err != nil {
		return err
	}
	if err := h.d.Teams.Delete(r.Context(), id, version); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}
