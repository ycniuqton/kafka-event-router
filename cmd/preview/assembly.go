package main

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/library/resolver"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/app"
	"github.com/eventrouter/router/internal/platform/controlplane"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/metrics"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/preview/httpapi"
	"github.com/eventrouter/router/internal/sandboxinfo"
	pkgconfig "github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/kafka"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// Assembly composes the preview service graph from settings and shared infrastructure.
type Assembly struct {
	deps     *app.Deps
	registry *metrics.Registry
	settings preview.Settings
	now      func() time.Time
}

// Built is what the HTTP layer mounts.
type Built struct {
	Mounter *api.Mounter
	Module  api.Module
	Scripts preview.ScriptRuntimeProvider
}

// Build wires stores, sources, runner, service and handlers.
func (a Assembly) Build(ctx context.Context) (Built, error) {
	sessions, err := a.sessions()
	if err != nil {
		return Built{}, err
	}
	link := a.controlPlane()
	sources, err := a.sources(link)
	if err != nil {
		return Built{}, err
	}
	salts, err := preview.NewSaltProviderRegistry(link.Salts()).Build(a.settings.Salts)
	if err != nil {
		return Built{}, err
	}
	builder, scripts, err := a.pipelineBuilder(ctx, salts)
	if err != nil {
		return Built{}, err
	}
	registries, err := preview.NewRegistryProviderRegistry(link.specs(), a.now).Build(a.settings.SchemaRegistry)
	if err != nil {
		return Built{}, err
	}
	builder.Registries = registries
	testTopics, err := a.testTopics(ctx, link)
	if err != nil {
		return Built{}, err
	}
	decoder := preview.NewSampleDecoder(registries)
	service := preview.NewService(preview.ServiceDeps{
		Sources: sources, Sessions: sessions,
		Runner: preview.PreviewRunner{Builder: builder, Differ: preview.StructuralDiffer{}, Deadline: preview.RunDeadline.Duration(), Now: a.now},
		Views:  preview.SampleViewBuilder{Decoder: decoder},
		Schemas: preview.SchemaBuilder{Decoder: decoder, Inferrer: preview.NewSchemaInferrer(),
			Merger: preview.RegistrySchemaMerger{Sources: preview.DefaultRegistryFieldSources(), Decoder: decoder}},
		Masks: preview.MaskPolicy{Maskers: preview.DefaultMaskerManager()}, TestTopics: testTopics,
		Metrics: preview.NewMetrics(a.registry.Registerer()), Log: a.deps.Log, Now: a.now, SchemaJobs: a.settings.SchemaJobs,
	})
	responder := api.NewResponder(preview.NewProblemCatalog(), a.deps.Log)
	mounter := api.NewMounter(responder, a.globals(responder), []api.RouteMiddleware{
		httpapi.DirectiveStage{Codec: preview.NewDirectiveCodec(a.settings.DirectiveSecret, a.now), Responder: responder},
	})
	module := httpapi.NewModule(httpapi.Deps{Service: service, Responder: responder, Gate: httpapi.NewRunGate(a.settings.MaxRuns)})
	return Built{Mounter: mounter, Module: module, Scripts: scripts}, nil
}

// testTopics builds the test-topic writer and starts the janitor (FR-22) for the configured registry.
// pipelineBuilder starts the configured script runtime and the run compiler, both resolving library pins.
func (a Assembly) pipelineBuilder(ctx context.Context, salts host.SaltProvider) (preview.PipelineBuilder, preview.ScriptRuntimeProvider, error) {
	sandboxCfg, err := sandboxinfo.NewSettingsLoader().Load()
	if err != nil {
		return preview.PipelineBuilder{}, nil, err
	}
	library := a.library()
	scripts, err := preview.NewScriptRuntimeRegistry(salts, sandboxCfg).WithLibrary(a.libraryTables(library)).Build(ctx, a.settings.Scripts)
	if err != nil {
		return preview.PipelineBuilder{}, nil, err
	}
	builder, err := preview.NewPipelineBuilder(compile.NewDefaultCompiler(), scripts, salts)
	builder.Library = library
	return builder, scripts, err
}

// library resolves pinned library versions from the shared database (nil without one).
func (a Assembly) library() pkgconfig.LibraryResolver {
	if a.deps.DB == nil {
		return nil
	}
	return resolver.New(a.deps.DB)
}

// libraryTables lets ctx.lookup read the pinned tables through library (nil without one).
func (Assembly) libraryTables(library pkgconfig.LibraryResolver) host.LibraryTables {
	if library == nil {
		return nil
	}
	return enginebind.LibraryTables{Resolver: library}
}

func (a Assembly) testTopics(ctx context.Context, link ControlPlaneLink) (preview.TestTopicWriter, error) {
	reg := preview.NewTestTopicRegistries()
	reg.Register(preview.TestTopicsMemory, func() (preview.TestTopicRegistry, error) { return preview.NewMemoryTestTopicRegistry(), nil })
	reg.Register(preview.TestTopicsPostgres, func() (preview.TestTopicRegistry, error) { return preview.NewPostgresTestTopicRegistry(a.deps.DB), nil })
	registry, err := reg.Build(a.settings.TestTopics)
	if err != nil {
		return preview.TestTopicWriter{}, err
	}
	clients := preview.SpecClients{Specs: link.specs(), Clients: kafka.DefaultClientFactoryRegistry()}
	janitor := preview.TestTopicJanitor{Registry: registry, Deleter: preview.AdminTopicDeleter{Clients: clients}, Lock: a.janitorLock(), Now: a.now, Log: a.deps.Log}
	go janitor.Run(ctx)
	return preview.TestTopicWriter{Clients: clients, Registry: registry, Now: a.now}, nil
}

// janitorLock shares the lock through Redis when sessions do (several replicas), else runs locally.
func (a Assembly) janitorLock() preview.JanitorLock {
	locks := map[preview.StoreKind]func() preview.JanitorLock{
		preview.StoreRedis:  func() preview.JanitorLock { return preview.RedisJanitorMutex{Client: a.deps.Redis} },
		preview.StoreMemory: func() preview.JanitorLock { return preview.LocalJanitorMutex{} },
	}
	if build, ok := locks[a.settings.SessionStore]; ok {
		return build()
	}
	return preview.LocalJanitorMutex{}
}

func (a Assembly) sessions() (*preview.SessionStoreManager, error) {
	reg := preview.NewSessionStoreRegistry()
	reg.Register(preview.StoreMemory, func() (preview.SessionStore, error) { return preview.NewMemorySessionStore(a.now), nil })
	reg.Register(preview.StoreRedis, func() (preview.SessionStore, error) { return preview.NewRedisSessionStore(a.deps.Redis), nil })
	store, err := reg.Build(a.settings.SessionStore)
	if err != nil {
		return nil, err
	}
	return preview.NewSessionStoreManager(store, a.now), nil
}

func (a Assembly) sources(link ControlPlaneLink) (*preview.SampleSourceManager, error) {
	clients, err := preview.NewSamplingClientRegistry(link.Sampling()).Build(a.settings.KafkaSampler)
	if err != nil {
		return nil, err
	}
	sampling := preview.KafkaSampling{Clients: clients}
	return preview.NewSampleSourceManager(
		preview.KafkaLatestSource{Sampling: sampling, Planner: kafka.LatestOffsetPlanner{}},
		preview.KafkaTimeRangeSource{Sampling: sampling, Planner: kafka.TimeRangeOffsetPlanner{}},
		preview.UploadSource{Clock: a.now},
		preview.NewDemoSource(a.now),
	), nil
}

// controlPlane builds the API internal client with the workload credential of ROUTER_WORKLOAD_AUTH;
// a failure is logged once and only breaks Kafka loads and control-plane salts (uploads keep working).
func (a Assembly) controlPlane() ControlPlaneLink {
	cp, err := controlplane.NewClient(a.deps.Config.ControlPlane, a.deps.Config.Internal.WorkloadAuth)
	if err != nil {
		a.deps.Log.Warn(string(preview.LogControlPlaneUnavailable), string(logging.AttrError), err)
	}
	return ControlPlaneLink{Client: cp, Err: err}
}

// ControlPlaneLink is the preview's view of the API internal listener: connection specs and team salts.
type ControlPlaneLink struct {
	Client *controlplane.Client
	Err    error
}

// Sampling opens group-less sampling clients for connection specs read from the API.
func (l ControlPlaneLink) Sampling() preview.FranzSamplingFactory {
	return preview.FranzSamplingFactory{Specs: l.specs(), Clients: kafka.DefaultClientFactoryRegistry()}
}

func (l ControlPlaneLink) specs() preview.ConnectionSpecSource {
	if l.Err != nil {
		return preview.UnreachableSpecs{Err: l.Err}
	}
	return preview.ControlPlaneSpecs{Fetcher: l.Client}
}

// Salts reads team hash salts from the API (selected by ROUTER_PREVIEW_SALTS).
func (l ControlPlaneLink) Salts() host.SaltProvider {
	if l.Err != nil {
		return preview.UnreachableSalts{Err: l.Err}
	}
	return preview.NewControlPlaneSalts(l.Client)
}

func (a Assembly) globals(responder *api.Responder) []api.GlobalMiddleware {
	return []api.GlobalMiddleware{
		api.RequestIDMiddleware{},
		api.RecoverMiddleware{Responder: responder},
		api.MetricsMiddleware{Collector: metrics.NewHTTPCollector(a.registry.Registerer())},
		api.LoggingMiddleware{Log: a.deps.Log},
		api.NoStoreMiddleware{},
	}
}
