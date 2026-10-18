package dlq

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// AssemblyDeps are the ports the feature is built from (cmd/api supplies the adapters); nil Registries
// keep plain payloads, nil Visibility shows raw payloads, nil Specs disables demo seeding.
type AssemblyDeps struct {
	Settings   Settings
	Routes     RouteSource
	Clients    ClientOpener
	Specs      SpecLookup
	Redis      redis.UniversalClient
	Preview    PreviewForwarder
	Compiler   engine.Compiler
	Scripts    ScriptRuntimes
	Salts      ctxhelpers.SaltProvider
	Registries codec.RegistryProvider
	Library    config.LibraryResolver
	Visibility SampleVisibility
	Metrics    prometheus.Registerer
	Audit      AuditTrail
	Log        *slog.Logger
}

// Assembly composes the Manager from registries selected by Settings.
type Assembly struct {
	d AssemblyDeps
}

// NewAssembly wraps deps.
func NewAssembly(d AssemblyDeps) Assembly { return Assembly{d: d} }

// Feature is the assembled dead-letters feature: the Manager façade and the state janitor.
type Feature struct {
	Manager *Manager
	Janitor StateJanitor
}

// Build returns the Manager; root bounds the state tails and producer clients.
func (a Assembly) Build(root context.Context) (*Manager, error) {
	f, err := a.BuildFeature(root, AlwaysLeader{})
	return f.Manager, err
}

// BuildFeature composes every registry-selected component; leader elects the janitor's instance.
func (a Assembly) BuildFeature(root context.Context, leader Leadership) (Feature, error) {
	state, err := a.stateRegistry(root).Build(a.d.Settings.StateBackend)
	if err != nil {
		return Feature{}, err
	}
	executor, err := a.executors().Build(a.d.Settings.Executor)
	if err != nil {
		return Feature{}, err
	}
	reader, gate := KafkaReader{Source: FranzSource{Clients: a.d.Clients}}, StateGate{Store: state}
	locator := DemoSeedingLocator{Inner: StoreLocator{Routes: a.d.Routes}, Seeder: a.demoSeeder(), Log: a.d.Log}
	m := NewManager(ManagerDeps{
		Routes: a.d.Routes, Locator: locator, Reader: reader, State: state,
		Actions: []ActionHandler{RetryAction{Gate: gate, Lock: a.lock(), Executor: executor}, DiscardAction{Gate: gate}, RestoreAction{Gate: gate}},
		Exports: NewExportManager(JSONArrayExporter{}, NDJSONExporter{}), Views: a.views(), Payloads: a.payloads(),
		Studio: PreviewStudioLinker{Preview: a.d.Preview}, Summaries: a.summaries(), Audit: a.d.Audit,
		Metrics: NewMetrics(a.d.Metrics), Log: a.d.Log, Now: time.Now,
	})
	janitor := StateJanitor{Routes: a.d.Routes, Locator: locator, Reader: reader, State: state, Leader: leader, Log: a.d.Log}
	return Feature{Manager: m, Janitor: janitor}, nil
}

// demoSeeder selects the demo seeding policy (seeding needs connection specs to recognise demo clusters).
func (a Assembly) demoSeeder() DemoSeeder {
	if a.d.Specs == nil {
		return NoDemoSeeder{}
	}
	return DemoSeederRegistry{
		DemoSeedOn: func() DemoSeeder {
			return &KafkaDemoSeeder{Specs: a.d.Specs, Clients: a.d.Clients, Source: FranzSource{Clients: a.d.Clients}, Routes: a.d.Routes, Now: time.Now}
		},
	}.Build(a.d.Settings.DemoSeed)
}

func (a Assembly) views() ViewPolicy {
	if a.d.Visibility == nil {
		return PassThroughViewPolicy{}
	}
	return MaskingViewPolicy{Visibility: a.d.Visibility, Masker: TypePreservingMasker{}}
}

func (a Assembly) payloads() PayloadDecoder {
	if a.d.Registries == nil {
		return PlainPayloads{}
	}
	return SchemaPayloads{Codecs: codec.DefaultFactoryManager(), Registries: a.d.Registries}
}

func (a Assembly) stateRegistry(root context.Context) *StateRegistry {
	r := NewStateRegistry()
	r.Register(StateBackendKafka, func() StateStore { return NewCompactedTopicStore(root, a.d.Clients, a.d.Log) })
	r.Register(StateBackendMemory, func() StateStore { return NewMemoryStateStore() })
	return r
}

func (a Assembly) executors() *ExecutorRegistry {
	cache := NewPipelineCache(a.d.Compiler, a.d.Scripts, a.d.Salts).WithRegistries(a.d.Registries).WithLibrary(a.d.Library)
	producer := NewFranzProducer(a.d.Clients)
	return NewExecutorRegistry(InProcessExecutor{Pipelines: cache, Producer: producer}, PreviewExecutor{Preview: a.d.Preview, Producer: producer})
}

func (a Assembly) lock() RetryLock {
	if a.d.Redis == nil {
		return NewMemoryRetryLock(time.Now)
	}
	return RedisRetryLock{Client: a.d.Redis}
}

func (a Assembly) summaries() SummaryCache {
	if a.d.Redis == nil {
		return NoSummaryCache{}
	}
	return RedisSummaryCache{Client: a.d.Redis}
}
