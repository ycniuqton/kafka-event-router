package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/assist"
	"github.com/eventrouter/router/internal/assist/assembly"
	assisthttp "github.com/eventrouter/router/internal/assist/http"
	"github.com/eventrouter/router/internal/assist/mock"
	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/library"
	"github.com/eventrouter/router/internal/library/importers"
	"github.com/eventrouter/router/internal/library/libraryhttp"
	"github.com/eventrouter/router/internal/library/testcases"
	"github.com/eventrouter/router/internal/library/testcaseshttp"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/ratelimit"
	"github.com/eventrouter/router/internal/preview/gateway"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// StudioLog names the transform-studio wiring log events.
type StudioLog string

const LogStudioDisabled StudioLog = "transform-studio module disabled"

// studioProviders registers every LLMProvider implementation; ROUTER_ASSIST_PROVIDER picks one.
func studioProviders() *assist.ProviderRegistry {
	reg := assist.NewProviderRegistry()
	mock.Register(reg)
	return reg
}

// studioModule is the transform-studio registration point: /api/v1/assist/*, /api/v1/preview/infer-mapping,
// the library (/api/v1/transforms, /api/v1/lookup-tables) and test cases of routes and transforms.
// A configuration error disables the module (logged) instead of failing the API.
func (s *apiService) studioModule() api.Module {
	mods, err := s.studioModules()
	if err != nil {
		s.wiring.Deps.Log.Error(string(LogStudioDisabled), string(logging.AttrError), err)
		return api.Modules{}
	}
	return mods
}

// studioModules builds every transform-studio handler.
func (s *apiService) studioModules() (api.Modules, error) {
	settings, err := assist.NewSettingsLoader().Load()
	if err != nil {
		return nil, err
	}
	deps, err := s.assistDeps(settings)
	if err != nil {
		return nil, err
	}
	c, err := assembly.Build(deps)
	if err != nil {
		return nil, err
	}
	lib, libMod, err := s.libraryModule()
	if err != nil {
		return nil, err
	}
	s.wiring.registerStudioAuditActions()
	handlers := append(c.Module.Handlers(), libMod.Handlers()...)
	return api.Modules(append(handlers, s.testCasesModule(lib.Owner(library.KindTransform)).Handlers()...)), nil
}

// assistDeps adds the shared (Redis) limiter, live-preview's sample masking and Prometheus metrics.
func (s *apiService) assistDeps(settings assist.Settings) (assembly.Deps, error) {
	w := s.wiring
	limiter, err := ratelimit.NewRegistry().Build(w.Deps.Config.RateLimit, w.Deps.Redis)
	if err != nil {
		return assembly.Deps{}, err
	}
	masker, err := w.previewSampleMasker()
	if err != nil {
		return assembly.Deps{}, err
	}
	return assembly.Deps{Settings: settings, Providers: studioProviders(), Responder: w.Responder, Now: time.Now,
		Limiter: assist.NewSharedLimiter(limiter, settings.RatePerMinute), Samples: assistSamples{masker: masker},
		Metrics: assist.NewPromMetrics(s.registry.Registerer()), Toggle: assistToggle(w)}, nil
}

// assistToggle is access-control's workspace "AI assist" setting (nil when access control has no settings).
func assistToggle(w *Wiring) assisthttp.WorkspaceToggle {
	if w.Access == nil || w.Access.Settings == nil {
		return nil
	}
	return w.Access.Settings
}

// assistSamples adapts live-preview's AssistSampleMasker to the assistant's SampleMasker port.
type assistSamples struct {
	masker gateway.AssistSampleMasker
}

// Mask implements assisthttp.SampleMasker.
func (a assistSamples) Mask(ctx context.Context, p auth.Principal, connectionID string, samples []json.RawMessage) []json.RawMessage {
	if len(samples) == 0 {
		return nil
	}
	return a.masker.Mask(ctx, p, connectionID, samples).Samples
}

// libraryModule serves the transform library over the shared Postgres store; publishes compile with the
// test cases' JS sandbox and library resolver (nested library steps).
func (s *apiService) libraryModule() (*library.Service, api.Module, error) {
	checker, err := assist.NewStepChecker()
	if err != nil {
		return nil, nil, err
	}
	testCasesFeature.build(s.wiring)
	compiler := library.EngineStepCompiler{Engine: compile.NewDefaultCompiler(),
		Options: libraryCompileOptions{scripts: testCasesFeature.scripts, library: testCasesFeature.library}}
	db := s.wiring.Deps.DB
	svc := library.NewService(library.ServiceDeps{Kinds: library.DefaultKindManager(checker, library.DefaultLimits, compiler),
		Repo: library.NewRepository(db), Usages: library.NewUsageRepository(db), Importers: importers.Register(library.DefaultLimits),
		Tx: s.wiring.Tx, Limits: library.DefaultLimits, Metrics: library.NewPromPublishMetrics(s.registry.Registerer())})
	return svc, libraryhttp.NewModule(libraryhttp.Deps{Service: svc, Pages: s.wiring.Pages, Responder: s.wiring.Responder}), nil
}

// libraryCompileOptions are the compile options of library publish checks (sandbox in test scope).
type libraryCompileOptions struct {
	scripts testcases.ScriptRuntimes
	library config.LibraryResolver
}

// Options implements library.CompileOptionsSource.
func (o libraryCompileOptions) Options(team uuid.UUID) engine.CompileOptions {
	target := testcases.RunTarget{TeamID: team.String(), RouteID: string(library.CheckRef), RouteName: string(library.CheckName)}
	return engine.CompileOptions{Scripts: o.scripts.Runtime(target), Library: o.library, Helpers: ctxhelpers.Set{}.WithDefaults(),
		Meta: engine.RouteMeta{RouteID: target.RouteID, Name: target.RouteName}}
}

// registerStudioAuditActions adds the assistant's audit actions to access-control's catalog.
func (w *Wiring) registerStudioAuditActions() {
	if w.Access == nil || w.Access.Catalog == nil {
		return
	}
	w.Access.Catalog.Register(assisthttp.ActionProposed, audit.ActionMeta{Category: audit.CategoryChange})
	for _, action := range append(libraryhttp.AuditActions(), testcaseshttp.AuditActions()...) {
		w.Access.Catalog.Register(action, audit.ActionMeta{Category: audit.CategoryChange})
	}
}
