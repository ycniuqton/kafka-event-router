package main

import (
	"context"
	"fmt"
	"time"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/auth/dev"
	"github.com/eventrouter/router/internal/auth/oidc"
	"github.com/eventrouter/router/internal/auth/session"
	"github.com/eventrouter/router/internal/platform/config"
)

// AuthAssembly builds the session store, authenticators and login flows selected by ROUTER_AUTH_MODES.
type AuthAssembly struct {
	w     *Wiring
	ttls  session.TTLs
	flows map[config.AuthMode]func() (auth.LoginFlow, error)
}

// NewAuthAssembly prepares the assembly over the wiring.
func NewAuthAssembly(w *Wiring) *AuthAssembly {
	cfg := w.Deps.Config.Auth
	a := &AuthAssembly{w: w, ttls: session.TTLs{Idle: cfg.SessionIdleTTL, Absolute: cfg.SessionMaxTTL}}
	a.flows = map[config.AuthMode]func() (auth.LoginFlow, error){
		config.AuthModeOIDC: a.oidcFlow,
		config.AuthModeDev:  a.devFlow,

		config.AuthModePassword: PasswordAssembly{w: w}.Flow,
	}
	return a
}

// Build returns the login manager and the request authenticator manager.
func (a *AuthAssembly) Build(ctx context.Context) (*auth.LoginManager, *auth.Manager, error) {
	w := a.w
	w.Cookies = auth.NewCookieJar(w.Deps.Config.Auth.CookieSecure, a.ttls.Absolute)
	w.Sessions = session.NewRedisStore(w.Deps.Redis, a.ttls, time.Now)
	registry := auth.NewRegistry()
	registry.Register(auth.AuthenticatorSession, func() (auth.Authenticator, error) {
		return session.NewCookieAuthenticator(w.Sessions, w.Identity.Users, w.Cookies, a.ttls, time.Now), nil
	})
	registry.Register(auth.AuthenticatorAPIToken, w.tokenAuthenticator)
	manager, err := registry.Build(w.Deps.Config.Auth.Modes)
	if err != nil {
		return nil, nil, err
	}
	flows, err := a.loginFlows()
	if err != nil {
		return nil, nil, err
	}
	logins := auth.NewLoginManager(auth.LoginDeps{
		Users: w.Identity.Users, Sessions: w.Sessions, Cookies: w.Cookies, Tx: w.Tx, Now: time.Now,
		Hook:    auth.NewFirstUserAdminHook(w.Identity.Users, w.Deps.Config.Auth.Dev.TeamName, w.Deps.Audit),
		Auditor: auth.NewLoginAuditor(w.Deps.Audit, w.Deps.Log), Tracker: w.Identity.Users,
	}, flows...)
	return logins, manager, nil
}

func (a *AuthAssembly) loginFlows() ([]auth.LoginFlow, error) {
	var flows []auth.LoginFlow
	for _, mode := range a.w.Deps.Config.Auth.Modes {
		build, ok := a.flows[mode]
		if !ok {
			continue
		}
		flow, err := build()
		if err != nil {
			return nil, fmt.Errorf("auth: %s login: %w", mode, err)
		}
		flows = append(flows, flow)
	}
	return flows, nil
}

func (a *AuthAssembly) oidcFlow() (auth.LoginFlow, error) {
	cfg := a.w.Deps.Config.Auth
	sealer, err := oidc.NewStateSealer(cfg.CookieSecret)
	if err != nil {
		return nil, err
	}
	discovery := oidc.NewDiscovery(cfg.OIDC.Issuer, time.Now)
	if a.interactiveModes() == 1 {
		a.w.Deps.Health.AddCheck(discovery)
	}
	return oidc.NewFlow(cfg.OIDC, discovery, sealer, a.w.Cookies, time.Now), nil
}

func (a *AuthAssembly) devFlow() (auth.LoginFlow, error) {
	w := a.w
	seeder := &dev.Seeder{Users: w.Identity.Users, Teams: w.Identity.Teams, Bindings: w.Identity.RoleBindings,
		Tx: w.Tx, Recorder: w.Deps.Audit, Config: w.Deps.Config.Auth.Dev}
	return dev.NewFlow(seeder, w.Deps.Config.Auth.Dev), nil
}

// interactiveModes counts the configured modes that have a login flow (api_token has none).
func (a *AuthAssembly) interactiveModes() int {
	n := 0
	for _, mode := range a.w.Deps.Config.Auth.Modes {
		if _, ok := a.flows[mode]; ok {
			n++
		}
	}
	return n
}
