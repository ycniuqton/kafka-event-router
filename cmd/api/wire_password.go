package main

import (
	"context"

	"github.com/eventrouter/router/internal/auth"
	authhandlers "github.com/eventrouter/router/internal/auth/handlers"
	"github.com/eventrouter/router/internal/auth/password"
	passwordhttp "github.com/eventrouter/router/internal/auth/password/httpapi"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/ratelimit"
	"github.com/eventrouter/router/internal/rbac"
)

// PasswordWiring holds password login's components (nil on Wiring when the mode is off).
type PasswordWiring struct {
	Creds *password.Credentials
	Flow  *password.Flow
}

// PasswordAssembly builds password login: credentials manager, throttle, flow, admin seed, HTTP members.
type PasswordAssembly struct {
	w *Wiring
}

// Flow builds the parts and returns the login flow (called by AuthAssembly when "password" is in ROUTER_AUTH_MODES).
func (a PasswordAssembly) Flow() (auth.LoginFlow, error) {
	w, cfg := a.w, a.w.Deps.Config
	limiter, err := ratelimit.NewRegistry().Build(cfg.RateLimit, w.Deps.Redis)
	store := password.NewPostgresCredentialStore(w.Deps.DB)
	if err != nil {
		return nil, err
	}
	creds := password.NewCredentials(password.CredentialsDeps{Store: store,
		Hashers: password.NewHasherSet(password.NewArgon2idHasher(password.DefaultArgon2Params)), Policy: password.DefaultPolicy,
		Tx: w.Tx, Trail: password.NewTrail(w.Deps.Audit)})
	throttle := password.NewRedisThrottle(w.Deps.Redis, limiter, password.ThrottlePolicy{MaxFailures: cfg.Auth.Password.MaxFailures,
		Lockout: cfg.Auth.Password.Lockout, IPPerMinute: cfg.Auth.Password.IPPerMinute})
	flow, err := password.NewFlow(store, creds.Hashers(), throttle)
	if err != nil {
		return nil, err
	}
	w.Password = &PasswordWiring{Creds: creds, Flow: flow}
	return flow, nil
}

// buildPassword seeds the first admin and registers the /auth/me member; it runs after buildAccess.
func (w *Wiring) buildPassword(ctx context.Context) error {
	if w.Password == nil {
		return nil
	}
	w.MeContributors = append(w.MeContributors, passwordhttp.MeContributor{})
	seeder := &password.AdminSeeder{Creds: w.Password.Creds, Accounts: password.NewPostgresSeedAccounts(w.Deps.DB),
		Teams: w.Identity.Teams, Bindings: w.Identity.RoleBindings, Tx: w.Tx, Recorder: w.Deps.Audit,
		Config: w.Deps.Config.Auth.Password, TeamName: w.Deps.Config.Auth.Dev.TeamName, Log: w.Deps.Log}
	return seeder.Run(ctx)
}

// passwordModule returns the login/change handlers, or nothing when password mode is off.
func (w *Wiring) passwordModule() api.Modules {
	if w.Password == nil {
		return nil
	}
	return api.Modules(passwordhttp.NewModule(passwordhttp.Deps{Logins: w.Logins, Creds: w.Password.Creds, Responder: w.Responder}).Handlers())
}

// passwordConfig returns the /auth/config contributors of password mode.
func (w *Wiring) passwordConfig() []authhandlers.ConfigContributor {
	if w.Password == nil {
		return nil
	}
	return []authhandlers.ConfigContributor{passwordhttp.ConfigContributor{Policy: w.Password.Creds.Policy()}}
}

// passwordUsers builds the admin service for POST /users and password resets (nil when password mode is off).
func (a *AccessAssembly) passwordUsers(p AccessParts, users rbac.UserStore, bindings rbac.BindingStore) *rbac.PasswordUserService {
	w := a.w
	if w.Password == nil {
		return nil
	}
	return rbac.NewPasswordUserService(rbac.PasswordUserDeps{Tx: w.Tx, Accounts: rbac.NewPostgresPasswordAccountStore(w.Deps.DB),
		Creds: w.Password.Creds, Users: users, Bindings: bindings, Teams: p.Teams, Assignable: p.Assignable,
		Sessions: w.Sessions, Trail: p.Trail})
}

// passwordGate is the route stage blocking everything but the change allow list during a forced change.
func (w *Wiring) passwordGate() api.RouteMiddleware {
	return password.GateStage{Allowed: password.ChangeAllowed, Responder: w.Responder}
}
