package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/rbac"
)

// GrantList is the body of GET /teams/{id}/grants.
type GrantList struct {
	Items []rbac.ConnectionGrant `json:"items"`
}

// ListGrantsHandler serves GET /teams/{id}/grants (any member: grants are shown read-only next to connections).
type ListGrantsHandler struct{ d Deps }

func (h *ListGrantsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathTeamGrants), Permission: rbac.PermTeamRead.Auth()}
}

func (h *ListGrantsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	team, err := UUIDReader{}.Path(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return err
	}
	items, err := h.d.Grants.List(r.Context(), team)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, GrantList{Items: items})
}

// PutGrantHandler serves PUT /teams/{id}/grants/{connectionId} (idempotent; workspace admins).
type PutGrantHandler struct{ d Deps }

func (h *PutGrantHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPut, Pattern: string(PathTeamGrant), Permission: rbac.PermGrantManage.Auth(),
		Mutating: true, AuditAction: audit.ActionGrantAdded}
}

func (h *PutGrantHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	team, conn, err := GrantKeyReader{}.Read(r)
	if err != nil {
		return err
	}
	g, err := h.d.Grants.Add(r.Context(), team, conn)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, g)
}

// DeleteGrantHandler serves DELETE /teams/{id}/grants/{connectionId}.
type DeleteGrantHandler struct{ d Deps }

func (h *DeleteGrantHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodDelete, Pattern: string(PathTeamGrant), Permission: rbac.PermGrantManage.Auth(),
		Mutating: true, AuditAction: audit.ActionGrantRemoved}
}

func (h *DeleteGrantHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	team, conn, err := GrantKeyReader{}.Read(r)
	if err != nil {
		return err
	}
	if err := h.d.Grants.Remove(r.Context(), team, conn); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// GrantKeyReader reads the (team id, connection id) path pair.
type GrantKeyReader struct{}

// Read parses both ids (404 when malformed).
func (GrantKeyReader) Read(r *http.Request) (uuid.UUID, uuid.UUID, error) {
	team, err := UUIDReader{}.Path(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	conn, err := UUIDReader{}.Path(chi.URLParam(r, string(ParamConnID)))
	return team, conn, err
}
