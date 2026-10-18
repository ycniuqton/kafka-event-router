package handlers

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/auth/oidc"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/problem"
)

// ConflictCode is shown when the email already belongs to another SSO subject.
type ConflictCode string

const (
	LoginConflict ConflictCode = "conflict"
	LoginDisabled ConflictCode = "disabled"
)

// ConfigResponse tells the login page which sign-in options exist.
type ConfigResponse struct {
	Modes        []auth.AuthMode `json:"modes"`
	OIDCLoginURL string          `json:"oidc_login_url,omitempty"`
	DevLoginURL  string          `json:"dev_login_url,omitempty"`
	Workspace    string          `json:"workspace"`
}

// ConfigHandler serves GET /auth/config (public).
type ConfigHandler struct{ d Deps }

func (h *ConfigHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathConfig), Auth: api.AuthNone}
}

func (h *ConfigHandler) Serve(w http.ResponseWriter, _ *http.Request) error {
	resp := ConfigResponse{Modes: h.d.Logins.Modes(), Workspace: h.d.Config.Workspace}
	for _, m := range resp.Modes {
		if m == config.AuthModeOIDC {
			resp.OIDCLoginURL = string(PathOIDCLogin)
		}
		if m == config.AuthModeDev {
			resp.DevLoginURL = string(PathDevLogin)
		}
	}
	return h.d.Responder.JSON(w, http.StatusOK, resp)
}

// OIDCLoginHandler serves GET /auth/oidc/login?return_to=… (302 to the IdP).
type OIDCLoginHandler struct{ d Deps }

func (h *OIDCLoginHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathOIDCLogin), Auth: api.AuthNone}
}

func (h *OIDCLoginHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	err := h.d.Logins.Begin(config.AuthModeOIDC, w, r)
	var loginErr *oidc.LoginError
	if errors.As(err, &loginErr) {
		LoginRedirect{}.Failure(w, r, string(loginErr.Code))
		return nil
	}
	return err
}

// OIDCCallbackHandler serves GET /auth/oidc/callback (302 to return_to, or /login?error=…).
type OIDCCallbackHandler struct{ d Deps }

func (h *OIDCCallbackHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathOIDCCallback), Auth: api.AuthNone}
}

func (h *OIDCCallbackHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := h.d.Logins.Complete(config.AuthModeOIDC, w, r)
	var loginErr *oidc.LoginError
	var classified problem.Classified
	switch {
	case err == nil:
		http.Redirect(w, r, id.ReturnTo, http.StatusFound)
	case errors.As(err, &loginErr):
		LoginRedirect{}.Failure(w, r, string(loginErr.Code))
	case errors.Is(err, auth.ErrUserDisabled):
		LoginRedirect{}.Failure(w, r, string(LoginDisabled))
	case errors.As(err, &classified) && classified.ProblemKind() == problem.KindConflict:
		LoginRedirect{}.Failure(w, r, string(LoginConflict))
	default:
		return err
	}
	return nil
}

// DevLoginHandler serves POST /auth/dev/login (local only).
type DevLoginHandler struct{ d Deps }

func (h *DevLoginHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathDevLogin), Auth: api.AuthNone}
}

func (h *DevLoginHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	if _, err := h.d.Logins.Complete(config.AuthModeDev, w, r); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// LogoutHandler serves POST /auth/logout (works even when the session already expired).
type LogoutHandler struct{ d Deps }

func (h *LogoutHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathLogout), Auth: api.AuthOptional}
}

func (h *LogoutHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	if err := h.d.Logins.Logout(w, r); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// LoginRedirect sends the browser back to the login page with a reason code.
type LoginRedirect struct{}

// Failure redirects to /login?error=<code>.
func (LoginRedirect) Failure(w http.ResponseWriter, r *http.Request, code string) {
	q := url.Values{string(ParamError): {code}}
	http.Redirect(w, r, string(PathLoginPage)+"?"+q.Encode(), http.StatusFound)
}
