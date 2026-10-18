// Package app is the common bootstrap for every binary: config, logging, shared infrastructure
// (Postgres, Redis, secrets, auth, audit), public + optional internal listeners, graceful shutdown.
package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/tracing"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/store"
)

// ExitCode is the process exit status returned by Run.
type ExitCode int

const (
	ExitOK      ExitCode = 0
	ExitFailure ExitCode = 1
)

// Service is implemented by each binary to attach its own HTTP routes.
type Service interface {
	Name() config.ServiceName
	Mount(r chi.Router, deps Deps)
}

// InfraConsumer is optionally implemented by services that need shared infrastructure opened.
type InfraConsumer interface {
	Infra() []Infra
}

// DepsBuilder is optionally implemented by services that build further deps (secrets, auth, audit, migrate).
type DepsBuilder interface {
	BuildDeps(ctx context.Context, deps *Deps) error
}

// InternalService is optionally implemented by services that serve the internal listener.
type InternalService interface {
	MountInternal(r chi.Router, deps Deps)
}

// Deps carries shared infrastructure handed to a Service at mount time; nil fields were not requested.
type Deps struct {
	Config  config.Config
	Log     *slog.Logger
	Health  *httpserver.Health
	DB      *store.DB
	Redis   redis.UniversalClient
	Secrets secrets.Manager
	Auth    *auth.Manager
	Audit   audit.Recorder
}

// Runner bootstraps and runs a Service until SIGINT/SIGTERM.
type Runner struct {
	service Service
}

// NewRunner creates a Runner for the given service.
func NewRunner(service Service) *Runner {
	return &Runner{service: service}
}

// Run loads config, opens infrastructure, builds the routers and serves until a termination signal.
func (r *Runner) Run() ExitCode {
	cfg, err := config.NewLoader().Load(r.service.Name())
	if err != nil {
		slog.Error("load config", string(logging.AttrError), err)
		return ExitFailure
	}
	log := logging.NewFactory(os.Stdout).New(cfg)
	log = slog.New(tracing.NewLogHandler(log.Handler()))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	tracer, err := tracing.NewRegistry().Setup(ctx, cfg)
	if err != nil {
		log.Error("tracing setup failed", string(logging.AttrError), err)
		return ExitFailure
	}
	defer r.shutdownTracing(tracer, cfg, log)

	deps, closer, err := r.prepare(ctx, cfg, log)
	defer closer.Close()
	if err != nil {
		log.Error("bootstrap failed", string(logging.AttrError), err)
		return ExitFailure
	}
	go deps.Health.Watch(ctx, time.Duration(httpserver.DefaultProbeInterval))
	listeners, err := r.listeners(deps)
	if err != nil {
		log.Error("tls setup failed", string(logging.AttrError), err)
		return ExitFailure
	}
	if err := listeners.Run(ctx); err != nil {
		log.Error("server stopped with error", string(logging.AttrError), err)
		return ExitFailure
	}
	log.Info("server stopped")
	return ExitOK
}

// shutdownTracing flushes pending spans within the shutdown timeout.
func (r *Runner) shutdownTracing(tracer *tracing.Provider, cfg config.Config, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := tracer.Shutdown(ctx); err != nil {
		log.Warn("tracing shutdown failed", string(logging.AttrError), err)
	}
}

func (r *Runner) prepare(ctx context.Context, cfg config.Config, log *slog.Logger) (Deps, *Closer, error) {
	deps := Deps{Config: cfg, Log: log, Health: httpserver.NewHealth()}
	closer := &Closer{}
	if consumer, ok := r.service.(InfraConsumer); ok {
		if err := (InfraOpener{}).Open(ctx, consumer.Infra(), &deps, closer); err != nil {
			return deps, closer, err
		}
	}
	if builder, ok := r.service.(DepsBuilder); ok {
		if err := builder.BuildDeps(ctx, &deps); err != nil {
			return deps, closer, err
		}
	}
	return deps, closer, nil
}

// listeners builds the public listener and, when configured, the internal one; both share TLSConfig
// (the internal one additionally verifies client certificates when a client CA is set).
func (r *Runner) listeners(deps Deps) (*Listeners, error) {
	tlsFactory := httpserver.NewTLSFactory(deps.Config.TLS)
	publicTLS, err := tlsFactory.Server(httpserver.RolePublic)
	if err != nil {
		return nil, err
	}
	ls := &Listeners{log: deps.Log}
	ls.Add(httpserver.NewServer(deps.Config.HTTP, r.publicRouter(deps), deps.Health, deps.Log).WithTLS(publicTLS))
	internal, ok := r.service.(InternalService)
	if !ok || deps.Config.Internal.Addr == "" {
		return ls, nil
	}
	internalTLS, err := tlsFactory.Server(httpserver.RoleInternal)
	if err != nil {
		return nil, err
	}
	router := chi.NewRouter()
	router.Use(middleware.RealIP, tracing.Middleware{}.Handler, middleware.Recoverer)
	internal.MountInternal(router, deps)
	cfg := deps.Config.HTTP
	cfg.Addr = deps.Config.Internal.Addr
	ls.Add(httpserver.NewServer(cfg, router, nil, deps.Log).WithTLS(internalTLS))
	return ls, nil
}

func (r *Runner) publicRouter(deps Deps) chi.Router {
	router := chi.NewRouter()
	router.Use(middleware.RealIP, tracing.Middleware{}.Handler, middleware.Recoverer)
	router.Get(string(httpserver.PathHealthz), deps.Health.Liveness)
	router.Get(string(httpserver.PathReadyz), deps.Health.Readiness)
	r.service.Mount(router, deps)
	return router
}
