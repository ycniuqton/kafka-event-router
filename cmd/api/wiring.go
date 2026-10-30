package main

import (
	"context"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	authhandlers "github.com/eventrouter/router/internal/auth/handlers"
	"github.com/eventrouter/router/internal/auth/session"
	"github.com/eventrouter/router/internal/auth/workload"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/app"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/metrics"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/secrets"
	secrethandlers "github.com/eventrouter/router/internal/secrets/handlers"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/store/identity"
)

// Wiring holds the API's composed components; feature folders read what they need from it.
type Wiring struct {
	Deps           *app.Deps
	Tx             *store.PgTxManager
	Notifier       *store.Notifier
	Identity       *identity.Repositories
	Cookies        *auth.CookieJar
	Sessions       *session.RedisStore
	Logins         *auth.LoginManager
	Authorizers    *auth.AuthorizerRegistry
	Responder      *api.Responder
	Pages          *api.PageParser
	AuditMW        *audit.Middleware
	Public         *api.Mounter
	Internal       *api.Mounter
	WebSockets     *api.WSUpgrader
	AuthModule     api.Module
	SecretsModule  api.Module
	Access         *AccessWiring
	Password       *PasswordWiring
	Platform       *PlatformWiring
	PlatformP3     *PlatformP3Wiring
	SecretParts    SecretsParts
	MeContributors MeContributors
}

// NewWiring composes everything in dependency order.
func NewWiring(ctx context.Context, deps *app.Deps, reg *metrics.Registry) (*Wiring, error) {
	w := &Wiring{Deps: deps, Tx: store.NewTxManager(deps.DB), Notifier: store.NewNotifier(deps.DB), Identity: identity.New(deps.DB)}
	steps := []func(context.Context) error{w.migrate, w.buildAudit, w.buildSecrets, w.buildPlatform, w.buildAuth, w.buildAccess, w.buildPassword, w.buildPlatformP3(reg), w.buildMounters(reg), w.buildModules}
	for _, step := range steps {
		if err := step(ctx); err != nil {
			return nil, err
		}
	}
	return w, nil
}

func (w *Wiring) migrate(ctx context.Context) error {
	if !w.Deps.Config.Database.MigrateOnStart {
		return nil
	}
	w.Deps.Log.Info("applying migrations", string(logging.AttrMigration), "up")
	return store.NewMigrator(w.Deps.DB, w.Deps.Config.Database.URL).Up(ctx)
}

func (w *Wiring) buildAudit(context.Context) error {
	w.Deps.Audit = audit.NewPostgresRecorder(w.Deps.DB)
	w.Responder = api.NewResponder(problem.NewCatalog(), w.Deps.Log)
	w.Pages = api.NewPageParser(store.NewCursorCodec(w.Deps.Config.HTTP.CursorSecret))
	w.AuditMW = audit.NewMiddleware(w.Deps.Audit, audit.NewDiffBuilder(logging.NewRedactor()), api.AuditContext, w.Deps.Log)
	return nil
}

func (w *Wiring) buildSecrets(ctx context.Context) error {
	parts, err := SecretsAssembly{}.Build(ctx, w.Deps.Config.Secrets, w.Deps.DB)
	if err != nil {
		return err
	}
	w.SecretParts, w.Deps.Secrets = parts, parts.Manager
	return nil
}

func (w *Wiring) buildAuth(ctx context.Context) error {
	logins, manager, err := NewAuthAssembly(w).Build(ctx)
	if err != nil {
		return err
	}
	w.Logins, w.Deps.Auth = logins, manager
	w.Authorizers = auth.NewAuthorizerRegistry()
	return nil
}

func (w *Wiring) buildMounters(reg *metrics.Registry) func(context.Context) error {
	return func(context.Context) error {
		authz, err := w.Authorizers.Build(w.Deps.Config.Auth.Authz)
		if err != nil {
			return err
		}
		w.Public = api.NewMounter(w.Responder, w.globalMiddlewares(metrics.NewHTTPCollector(reg.Registerer())), []api.RouteMiddleware{
			api.AuthenticateStage{Authenticator: w.Deps.Auth, Responder: w.Responder},
			w.passwordGate(),
			api.RateLimitStage{Guard: w.Platform.RateGuard, Responder: w.Responder},
			api.CSRFStage{CSRF: auth.NewCSRFMiddleware(w.Cookies), Responder: w.Responder},
			api.AuthorizeStage{Authorize: auth.NewAuthorizeMiddleware(authz), Responder: w.Responder},
			api.ScopeStage{Scopes: auth.NewScopeGuard(w.Platform.Scopes), Responder: w.Responder},
			api.AuditStage{Audit: w.AuditMW},
		})
		ws := w.Deps.Config.WebSocket
		w.WebSockets = api.NewWSUpgrader(api.NewWSOriginPolicy(ws.AllowedOrigins), ws.PingInterval, w.Responder)
		return w.buildInternalMounter()
	}
}

func (w *Wiring) buildInternalMounter() error {
	if w.Deps.Config.Internal.Addr == "" {
		return nil
	}
	workloads, err := workload.NewRegistry().Build(w.Deps.Config.Internal)
	if err != nil {
		return err
	}
	w.Internal = api.NewMounter(w.Responder, []api.GlobalMiddleware{
		api.RequestIDMiddleware{}, api.RecoverMiddleware{Responder: w.Responder}, api.LoggingMiddleware{Log: w.Deps.Log}, api.NoStoreMiddleware{},
	}, []api.RouteMiddleware{
		api.AuthenticateStage{Authenticator: workloads, Responder: w.Responder},
		api.AuditStage{Audit: w.AuditMW},
	})
	return nil
}

func (w *Wiring) globalMiddlewares(collector *metrics.HTTPCollector) []api.GlobalMiddleware {
	return []api.GlobalMiddleware{
		api.RequestIDMiddleware{},
		api.RecoverMiddleware{Responder: w.Responder},
		api.MetricsMiddleware{Collector: collector},
		api.LoggingMiddleware{Log: w.Deps.Log},
		api.NoStoreMiddleware{},
	}
}

func (w *Wiring) buildModules(context.Context) error {
	base := authhandlers.NewModule(authhandlers.Deps{Logins: w.Logins, Responder: w.Responder, Config: w.Deps.Config,
		Contributors: w.MeContributors, ConfigContributors: w.passwordConfig()})
	w.AuthModule = append(api.Modules(base.Handlers()), w.passwordModule()...)
	w.SecretsModule = secrethandlers.NewModule(secrets.NewAuditedReader(w.Deps.Secrets, w.Deps.Audit), w.Responder)
	return nil
}
