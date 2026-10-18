// Package handlers serves the auth endpoints: /auth/config, OIDC login/callback, dev login,
// logout and /api/v1/auth/me.
package handlers

import (
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
)

// Path names the auth routes.
type Path string

const (
	PathConfig       Path = "/auth/config"
	PathOIDCLogin    Path = "/auth/oidc/login"
	PathOIDCCallback Path = "/auth/oidc/callback"
	PathDevLogin     Path = "/auth/dev/login"
	PathLogout       Path = "/auth/logout"
	PathMe           Path = "/api/v1/auth/me"
	PathLoginPage    Path = "/login"
)

// LoginErrorParam names the query parameter the login page reads after a failed callback.
type LoginErrorParam string

const ParamError LoginErrorParam = "error"

// Deps groups what the auth handlers need.
type Deps struct {
	Logins       *auth.LoginManager
	Responder    *api.Responder
	Config       config.Config
	Contributors []MeContributor
}

// NewModule returns the auth module; the dev login handler is included only when the dev flow is enabled.
func NewModule(d Deps) api.Module {
	handlers := api.Modules{
		&ConfigHandler{d: d},
		&OIDCLoginHandler{d: d},
		&OIDCCallbackHandler{d: d},
		&LogoutHandler{d: d},
		&MeHandler{d: d},
	}
	for _, mode := range d.Logins.Modes() {
		if extra, ok := modeHandlers[mode]; ok {
			handlers = append(handlers, extra(d))
		}
	}
	return handlers
}

// modeHandlers lists handlers that exist only when their login mode is enabled.
var modeHandlers = map[auth.AuthMode]func(Deps) api.Handler{
	config.AuthModeDev: func(d Deps) api.Handler { return &DevLoginHandler{d: d} },
}
