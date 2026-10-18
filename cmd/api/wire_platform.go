package main

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/auth/apitoken"
	tokenhttp "github.com/eventrouter/router/internal/auth/apitoken/httpapi"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/ratelimit"
)

// PlatformWiring holds platform-core's P2 components: API tokens, token scopes and the rate limiter.
type PlatformWiring struct {
	TokenRepo *apitoken.PostgresRepository
	Tokens    *apitoken.Manager
	Usage     *apitoken.UsageBatcher
	Scopes    *auth.ScopeCatalog
	RateGuard *ratelimit.Guard
}

// PlatformAuditActions are the platform-core actions added to access-control's audit catalog.
var PlatformAuditActions = []audit.Action{apitoken.ActionCreated, apitoken.ActionRevoked}

// buildPlatform composes API tokens and the rate limiter; it runs before buildAuth (the token authenticator needs them).
func (w *Wiring) buildPlatform(ctx context.Context) error {
	limiter, err := ratelimit.NewRegistry().Build(w.Deps.Config.RateLimit, w.Deps.Redis)
	if err != nil {
		return err
	}
	rule := ratelimit.Rule{PerMinute: w.Deps.Config.RateLimit.PerMinute, Burst: w.Deps.Config.RateLimit.Burst}
	repo, scopes := apitoken.NewPostgresRepository(w.Deps.DB), auth.DefaultScopeCatalog()
	usage := apitoken.NewUsageBatcher(repo, apitoken.DefaultFlushInterval, w.Deps.Log)
	go usage.Run(ctx)
	w.Platform = &PlatformWiring{
		TokenRepo: repo, Usage: usage, Scopes: scopes,
		Tokens:    apitoken.NewManager(apitoken.ManagerDeps{Repo: repo, Tx: w.Tx, Scopes: scopes, Policy: apitoken.DefaultPolicy(), Now: time.Now}),
		RateGuard: ratelimit.NewGuard(limiter, rule, w.Deps.Log),
	}
	return nil
}

// tokenAuthenticator builds the api_token request authenticator (registered by AuthAssembly).
func (w *Wiring) tokenAuthenticator() (auth.Authenticator, error) {
	return apitoken.NewAuthenticator(w.Platform.TokenRepo, w.Identity.Users, w.Platform.Usage, time.Now), nil
}

// platformModule returns the /api/v1/auth/tokens module and registers its audit actions.
func (s *apiService) platformModule() api.Module {
	w := s.wiring
	if w.Access != nil && w.Access.Catalog != nil {
		for _, a := range PlatformAuditActions {
			w.Access.Catalog.Register(a, audit.ActionMeta{Category: audit.CategoryAdmin, Sensitive: true})
		}
	}
	return tokenhttp.NewModule(tokenhttp.Deps{Tokens: w.Platform.Tokens, Responder: w.Responder})
}
