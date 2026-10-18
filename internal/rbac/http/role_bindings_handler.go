package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/rbac"
)

// BindingList is the body of GET /role-bindings (with the roles assignable in this phase).
type BindingList struct {
	Items           []rbac.RoleBinding `json:"items"`
	AssignableRoles []rbac.Role        `json:"assignableRoles"`
}

// ListBindingsHandler serves GET /role-bindings?userId=&teamId=.
type ListBindingsHandler struct{ d Deps }

func (h *ListBindingsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathRoleBindings), Permission: rbac.PermUserRead.Auth()}
}

func (h *ListBindingsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	user, err := UUIDReader{}.Query(r, ParamUserID)
	if err != nil {
		return err
	}
	team, err := UUIDReader{}.Query(r, ParamTeamID)
	if err != nil {
		return err
	}
	items, err := h.d.Bindings.List(r.Context(), rbac.BindingFilter{UserID: user, TeamID: team})
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, BindingList{Items: items, AssignableRoles: h.d.Bindings.AssignableRoles()})
}

// PutBindingRequest is the body of PUT /role-bindings/{userId}/{teamId}.
type PutBindingRequest struct {
	Role rbac.Role `json:"role"`
}

// PutBindingHandler serves PUT /role-bindings/{userId}/{teamId} (If-Match required when the binding exists).
type PutBindingHandler struct{ d Deps }

func (h *PutBindingHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPut, Pattern: string(PathRoleBinding), Permission: rbac.PermTeamMembersManage.Auth(),
		Object: rbac.ObjectResolvers{}.PathTeam(string(ParamTeamID)), Mutating: true, AuditAction: audit.ActionRoleBindingSet, Versioned: true}
}

func (h *PutBindingHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	user, team, err := BindingKey{}.Read(r)
	if err != nil {
		return err
	}
	ifMatch, err := OptionalIfMatch{}.Read(r)
	if err != nil {
		return err
	}
	req, err := api.Decoder[PutBindingRequest]{}.Decode(w, r)
	if err != nil {
		return err
	}
	b, err := h.d.Bindings.Set(r.Context(), rbac.SetBinding{UserID: user, TeamID: team, Role: req.Role, IfMatch: ifMatch})
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, b.RowVersion, b)
}

// DeleteBindingHandler serves DELETE /role-bindings/{userId}/{teamId} (If-Match required; 409 last_admin).
type DeleteBindingHandler struct{ d Deps }

func (h *DeleteBindingHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodDelete, Pattern: string(PathRoleBinding), Permission: rbac.PermTeamMembersManage.Auth(),
		Object: rbac.ObjectResolvers{}.PathTeam(string(ParamTeamID)), Mutating: true, AuditAction: audit.ActionRoleBindingRemoved, Versioned: true}
}

func (h *DeleteBindingHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	user, team, err := BindingKey{}.Read(r)
	if err != nil {
		return err
	}
	ifMatch, err := OptionalIfMatch{}.Read(r)
	if err != nil {
		return err
	}
	if err := h.d.Bindings.Remove(r.Context(), user, team, ifMatch); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// BindingKey reads the (userId, teamId) path pair.
type BindingKey struct{}

// Read parses both ids (404 when malformed).
func (BindingKey) Read(r *http.Request) (uuid.UUID, uuid.UUID, error) {
	user, err := UUIDReader{}.Path(chi.URLParam(r, string(ParamUserID)))
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	team, err := UUIDReader{}.Path(chi.URLParam(r, string(ParamTeamID)))
	return user, team, err
}
