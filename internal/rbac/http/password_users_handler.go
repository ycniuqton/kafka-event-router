package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/rbac"
)

// PasswordPath names the password-administration routes (mounted only in password mode).
type PasswordPath string

const PathUserPasswordReset PasswordPath = "/api/v1/users/{id}/password-reset"

// CreateUserRequest is the body of POST /users.
type CreateUserRequest struct {
	Email          string    `json:"email"`
	DisplayName    string    `json:"displayName"`
	Password       string    `json:"password"`
	WorkspaceAdmin bool      `json:"workspaceAdmin"`
	TeamID         string    `json:"teamId"`
	Role           rbac.Role `json:"role"`
}

// CreateUserResponse returns the user and, once, the generated password.
type CreateUserResponse struct {
	User              rbac.User `json:"user"`
	GeneratedPassword string    `json:"generatedPassword,omitempty"`
}

// CreateUserHandler serves POST /users (workspace admins): a password user that must change its password.
type CreateUserHandler struct{ d Deps }

func (h *CreateUserHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathUsers), Permission: rbac.PermUserManage.Auth(),
		Mutating: true, AuditAction: audit.ActionUserCreated}
}

func (h *CreateUserHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	req, err := api.Decoder[CreateUserRequest]{}.Decode(w, r)
	if err != nil {
		return err
	}
	cmd := rbac.NewPasswordUser{Email: req.Email, DisplayName: req.DisplayName, Password: req.Password,
		WorkspaceAdmin: req.WorkspaceAdmin, Role: req.Role}
	if req.TeamID != "" {
		team, err := uuid.Parse(req.TeamID)
		if err != nil {
			return rbac.ErrUnknownTeam
		}
		cmd.TeamID = &team
	}
	u, assigned, err := h.d.Passwords.Create(r.Context(), cmd)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusCreated, u.RowVersion, CreateUserResponse{User: u, GeneratedPassword: assigned.Generated})
}

// ResetPasswordRequest is the optional body of POST /users/{id}/password-reset.
type ResetPasswordRequest struct {
	Password string `json:"password"`
}

// ResetPasswordResponse returns the generated password once.
type ResetPasswordResponse struct {
	GeneratedPassword string `json:"generatedPassword,omitempty"`
}

// ResetPasswordHandler serves POST /users/{id}/password-reset: forced change at next login, sessions revoked.
type ResetPasswordHandler struct{ d Deps }

func (h *ResetPasswordHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathUserPasswordReset), Permission: rbac.PermUserManage.Auth(),
		Mutating: true, AuditAction: audit.ActionUserPasswordReset}
}

func (h *ResetPasswordHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := UUIDReader{}.Path(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return err
	}
	req, err := api.Decoder[ResetPasswordRequest]{AllowEmpty: true}.Decode(w, r)
	if err != nil {
		return err
	}
	assigned, err := h.d.Passwords.Reset(r.Context(), id, req.Password)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, ResetPasswordResponse{GeneratedPassword: assigned.Generated})
}
