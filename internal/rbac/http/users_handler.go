package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/rbac"
)

// ListUsersHandler serves GET /users?q=&team=&limit=&cursor=.
type ListUsersHandler struct{ d Deps }

func (h *ListUsersHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathUsers), Permission: rbac.PermUserRead.Auth()}
}

func (h *ListUsersHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	page, err := h.d.Pages.Parse(r)
	if err != nil {
		return err
	}
	team, err := UUIDReader{}.Query(r, ParamTeam)
	if err != nil {
		return err
	}
	filter := rbac.UserFilter{Text: strings.TrimSpace(r.URL.Query().Get(string(ParamQuery))), TeamID: team}
	users, next, err := h.d.Users.List(r.Context(), filter, rbac.Page{Limit: page.Limit, After: page.After})
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, api.PageResponse[rbac.User]{Items: users, NextCursor: h.d.Pages.Next(next)})
}

// GetUserHandler serves GET /users/{id} with an ETag.
type GetUserHandler struct{ d Deps }

func (h *GetUserHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathUser), Permission: rbac.PermUserRead.Auth()}
}

func (h *GetUserHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := UUIDReader{}.Path(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return err
	}
	u, err := h.d.Users.Get(r.Context(), id)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, u.RowVersion, u)
}

// PatchUserRequest is the body of PATCH /users/{id}.
type PatchUserRequest struct {
	DisplayName    *string `json:"displayName"`
	WorkspaceAdmin *bool   `json:"workspaceAdmin"`
	Disabled       *bool   `json:"disabled"`
}

// PatchUserHandler serves PATCH /users/{id} (If-Match required; 409 last_admin).
type PatchUserHandler struct{ d Deps }

func (h *PatchUserHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPatch, Pattern: string(PathUser), Permission: rbac.PermUserManage.Auth(),
		Mutating: true, AuditAction: audit.ActionUserUpdated, Versioned: true}
}

func (h *PatchUserHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := UUIDReader{}.Path(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return err
	}
	version, err := api.Precondition{}.Require(r)
	if err != nil {
		return err
	}
	req, err := api.Decoder[PatchUserRequest]{}.Decode(w, r)
	if err != nil {
		return err
	}
	change := rbac.UserChange{DisplayName: req.DisplayName, WorkspaceAdmin: req.WorkspaceAdmin, Disabled: req.Disabled}
	u, err := h.d.Users.Update(r.Context(), id, change, version)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, u.RowVersion, u)
}

// InviteRequest is the body of POST /users/invitations.
type InviteRequest struct {
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	TeamID      string    `json:"teamId"`
	Role        rbac.Role `json:"role"`
}

// InviteUserHandler serves POST /users/invitations (workspace admins): a pending user with one team role.
type InviteUserHandler struct{ d Deps }

func (h *InviteUserHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathInvitations), Permission: rbac.PermUserManage.Auth(),
		Mutating: true, AuditAction: audit.ActionUserInvited}
}

func (h *InviteUserHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	req, err := api.Decoder[InviteRequest]{}.Decode(w, r)
	if err != nil {
		return err
	}
	team, err := uuid.Parse(req.TeamID)
	if err != nil {
		return rbac.ErrUnknownTeam
	}
	u, err := h.d.Inviter.Invite(r.Context(), rbac.Invite{Email: req.Email, DisplayName: req.DisplayName, TeamID: team, Role: req.Role})
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusCreated, u.RowVersion, u)
}
