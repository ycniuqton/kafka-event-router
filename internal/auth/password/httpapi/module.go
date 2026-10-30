// Package httpapi serves password login (POST /auth/password/login), the self-service password change
// (POST /api/v1/auth/password) and the /auth/me and /auth/config members of password mode.
package httpapi

import (
	"context"
	"net/http"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	authhandlers "github.com/eventrouter/router/internal/auth/handlers"
	"github.com/eventrouter/router/internal/auth/password"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
)

// Path names the password routes.
type Path string

const (
	PathLogin  Path = "/auth/password/login"
	PathChange Path = "/api/v1/auth/password"
)

// Deps groups what the password handlers need.
type Deps struct {
	Logins    *auth.LoginManager
	Creds     *password.Credentials
	Responder *api.Responder
}

// NewModule returns the login and change handlers (mounted only when password mode is enabled).
func NewModule(d Deps) api.Module {
	return api.Modules{&LoginHandler{d: d}, &ChangeHandler{d: d}}
}

// LoginHandler serves POST /auth/password/login (204 + session cookies).
type LoginHandler struct{ d Deps }

func (h *LoginHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathLogin), Auth: api.AuthNone}
}

func (h *LoginHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.d.Logins.Complete(config.AuthModePassword, w, r); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// ChangeRequest is the body of POST /api/v1/auth/password.
type ChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangeHandler serves POST /api/v1/auth/password: own change, then every session is replaced by a fresh one.
type ChangeHandler struct{ d Deps }

func (h *ChangeHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathChange), Mutating: true, AuditAction: audit.ActionPasswordChanged}
}

func (h *ChangeHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	p := auth.PrincipalFrom(r.Context())
	if p.Kind != auth.PrincipalUser {
		return auth.ErrForbidden
	}
	req, err := api.Decoder[ChangeRequest]{}.Decode(w, r)
	if err != nil {
		return err
	}
	if err := h.d.Creds.Change(r.Context(), p.UserID, req.CurrentPassword, req.NewPassword); err != nil {
		return err
	}
	if err := h.d.Logins.Reissue(w, r, config.AuthModePassword); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// MeKey names the /auth/me member contributed by password mode.
type MeKey string

const KeyMustChangePassword MeKey = "must_change_password"

// MeContributor adds must_change_password to /auth/me.
type MeContributor struct{}

// Key implements authhandlers.MeContributor.
func (MeContributor) Key() string { return string(KeyMustChangePassword) }

// Contribute implements authhandlers.MeContributor.
func (MeContributor) Contribute(_ context.Context, p auth.Principal) (any, error) {
	return p.MustChangePassword, nil
}

// ConfigContributor adds the password login URL and policy to /auth/config.
type ConfigContributor struct {
	Policy password.Policy
}

// Contribute implements authhandlers.ConfigContributor.
func (c ConfigContributor) Contribute(resp *authhandlers.ConfigResponse) {
	resp.PasswordLoginURL = string(PathLogin)
	resp.PasswordPolicy = &authhandlers.PasswordPolicyView{MinLength: c.Policy.MinLength}
}
