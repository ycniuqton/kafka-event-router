package main

import (
	"context"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/app"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/internal/platform/metrics"
)

// apiService wires the control-plane API: infrastructure, auth, secrets, audit and feature modules.
type apiService struct {
	registry *metrics.Registry
	wiring   *Wiring
}

func newAPIService() *apiService {
	return &apiService{registry: metrics.NewRegistry()}
}

func (s *apiService) Name() config.ServiceName { return config.ServiceAPI }

// Infra implements app.InfraConsumer.
func (s *apiService) Infra() []app.Infra {
	return []app.Infra{app.InfraPostgres, app.InfraRedis}
}

// BuildDeps implements app.DepsBuilder (migrate-on-start, secrets, auth, audit).
func (s *apiService) BuildDeps(ctx context.Context, deps *app.Deps) error {
	w, err := NewWiring(ctx, deps, s.registry)
	if err != nil {
		return err
	}
	s.wiring = w
	return nil
}

// Mount attaches /metrics and every public module behind the standard middleware chain.
func (s *apiService) Mount(r chi.Router, _ app.Deps) {
	r.Handle(string(httpserver.PathMetrics), s.registry.Handler())
	public := chi.NewRouter()
	s.wiring.Public.Install(public)
	modules := s.publicModules()
	s.wiring.Access.VerifyCoverage(s.wiring.Deps.Log, modules...)
	s.wiring.Public.Mount(public, modules...)
	r.Mount("/", public)
}

// MountInternal attaches the workload-authenticated internal modules.
func (s *apiService) MountInternal(r chi.Router, _ app.Deps) {
	if s.wiring.Internal == nil {
		return
	}
	s.wiring.Internal.Install(r)
	s.wiring.Internal.Mount(r, s.internalModules()...)
}

// publicModules lists every feature module served on /api/v1 and /auth (features append here).
func (s *apiService) publicModules() []api.Module {
	return []api.Module{s.wiring.AuthModule, s.platformModule(), s.wiring.Access.Module, s.wiring.previewModule(), s.connectionsModule(), s.lookupSourcesModule(), s.routesModule(), s.monitoringModule(), s.sandboxModule(), s.studioModule(), s.dlqModule(), s.runtimeModule(), s.approvalsModule(), s.auditExportModule()}
}

// internalModules lists the modules served on the internal listener.
func (s *apiService) internalModules() []api.Module {
	return []api.Module{s.wiring.SecretsModule, s.connectionsInternalModule(), s.lookupSourcesInternalModule(), s.sandboxInternalModule(), s.runtimeInternalModule(), s.libraryInternalModule()}
}
