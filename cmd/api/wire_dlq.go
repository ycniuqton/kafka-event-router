package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/connections/pgstore"
	"github.com/eventrouter/router/internal/dlq"
	dlqhttp "github.com/eventrouter/router/internal/dlq/httpapi"
	"github.com/eventrouter/router/internal/dlq/jobs"
	"github.com/eventrouter/router/internal/events"
	"github.com/eventrouter/router/internal/library/resolver"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/preview/gateway"
	"github.com/eventrouter/router/internal/sandboxinfo"
	"github.com/eventrouter/router/internal/scheduler"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/kafka"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// dlqWiring composes the dead-letters feature once per process.
type dlqWiring struct {
	once   sync.Once
	module api.Module
}

var dlqFeature = &dlqWiring{}

// dlqModule returns the /api/v1/routes/{id}/dlq* and /api/v1/dlq* module; a build error disables it (logged).
func (s *apiService) dlqModule() api.Module {
	dlqFeature.once.Do(func() {
		m, err := buildDLQ(s)
		if err != nil {
			s.wiring.Deps.Log.Error(string(dlq.LogDisabled), string(logging.AttrError), err)
			m = api.Modules{}
		}
		dlqFeature.module = m
	})
	return dlqFeature.module
}

// buildDLQ assembles the Manager from the shared wiring, starts the job runner and the state janitor,
// and returns the HTTP module.
func buildDLQ(s *apiService) (api.Module, error) {
	w := s.wiring
	settings, err := dlq.SettingsLoader{Lookup: os.LookupEnv}.Load()
	if err != nil {
		return nil, err
	}
	routesC, err := w.RoutesComponents()
	if err != nil {
		return nil, err
	}
	authz, err := w.Authorizers.Build(w.Deps.Config.Auth.Authz)
	if err != nil {
		return nil, err
	}
	root, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	salts, specs := sandboxinfo.NewTeamSalts(w.Deps.Secrets, w.Identity.Teams), dlqSpecs(w)
	feature, err := dlq.NewAssembly(dlq.AssemblyDeps{
		Settings: settings, Routes: dlq.RoutesRepoSource{Repos: routesC.Repos}, Clients: dlqClients(specs), Specs: specs,
		Redis: w.Deps.Redis, Preview: dlqPreview(w), Compiler: compile.NewDefaultCompiler(),
		Scripts: dlqScripts(root, w, settings, salts), Salts: salts, Metrics: s.registry.Registerer(),
		Registries: &codec.HTTPRegistryProvider{Source: &preview.SpecRegistryConfigs{Specs: specs, Now: time.Now}},
		Visibility: dlqVisibility(w), Audit: dlq.ScopeAuditTrail{}, Log: w.Deps.Log, Library: resolver.New(w.Deps.DB),
	}).BuildFeature(root, dlqLeader(s))
	if err != nil {
		return nil, err
	}
	dlqRegisterAudit(w)
	svc, err := dlqJobs(root, w, settings, feature.Manager)
	if err != nil {
		return nil, err
	}
	go feature.Janitor.Run(root)
	return dlqhttp.NewModule(dlqhttp.Deps{Manager: feature.Manager, Jobs: svc, Authorizer: authz, Recorder: w.Deps.Audit, Responder: w.Responder}), nil
}

// dlqSpecs resolves saved connection specs with decrypted secrets (internal/secrets).
func dlqSpecs(w *Wiring) dlq.ConnectionSpecs {
	repo := pgstore.NewConnectionRepository(w.Deps.DB, w.Tx)
	return dlq.ConnectionSpecs{Resolver: connections.NewSpecResolver(repo, connections.NewSecretVault(w.Deps.Secrets))}
}

// dlqClients opens franz-go clients for saved connections; demo connections use this process's demo cluster.
func dlqClients(specs dlq.ConnectionSpecs) dlq.ClientOpener {
	factory := kafka.NewFranzClientFactory(kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog(), kafka.DefaultPurposeDefaults())
	kinds := map[kafka.ClusterKind]dlq.KindOptions{kafka.ClusterDemo: dlq.SandboxKindOptions{Inner: kafka.DefaultDemoClientFactory()}}
	return dlq.SpecClientOpener{Specs: specs, Options: factory, Kinds: kinds}
}

// dlqVisibility masks payloads per access-control's SampleVisibility (fails closed without access wiring).
func dlqVisibility(w *Wiring) dlq.SampleVisibility {
	if w.Access == nil || w.Access.Samples == nil {
		return dlq.AlwaysMasked{}
	}
	return dlq.RBACVisibility{Access: w.Access.Samples}
}

// dlqRegisterAudit adds the feature's P2 audit actions (restore, jobs) to the catalog.
func dlqRegisterAudit(w *Wiring) {
	if w.Access == nil || w.Access.Catalog == nil {
		return
	}
	for action, meta := range dlq.CatalogActions {
		w.Access.Catalog.Register(action, meta)
	}
}

// dlqLeader elects the janitor's instance with its own advisory lock.
func dlqLeader(s *apiService) dlq.Leadership {
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: string(MetricDLQJanitorLeader)})
	if err := s.registry.Registerer().Register(gauge); err != nil {
		s.wiring.Deps.Log.Warn(string(dlq.LogDisabled), string(logging.AttrError), err)
	}
	return scheduler.PgLeaderElector{Pool: s.wiring.Deps.DB.Pool(), Key: scheduler.LockKey(dlq.StateJanitorLockKey), Gauge: gauge}
}

// DLQMetric names metrics registered by the dead-letters wiring.
type DLQMetric string

const MetricDLQJanitorLeader DLQMetric = "router_dlq_janitor_leader"

// dlqJobs builds the job service and starts this instance's runner (claims are SKIP LOCKED).
func dlqJobs(root context.Context, w *Wiring, settings dlq.Settings, m *dlq.Manager) (*jobs.Service, error) {
	repo, err := jobs.NewRepositoryRegistry().
		Register(dlq.JobStorePostgres, func() jobs.Repository { return jobs.NewPostgresRepository(w.Deps.DB) }).
		Register(dlq.JobStoreMemory, func() jobs.Repository { return jobs.NewMemoryRepository(time.Now) }).
		Build(settings.JobStore)
	if err != nil {
		return nil, err
	}
	notifier := jobs.Notifiers{jobs.LogNotifier{Log: w.Deps.Log}, jobs.BusNotifier{Bus: w.dlqJobPublisher(), Log: w.Deps.Log}, dlqJobAudit(w, m)}
	runner := &jobs.Runner{Repo: repo, Pages: m, Notifier: notifier, Log: w.Deps.Log, Now: time.Now}
	go runner.Run(root)
	return &jobs.Service{Repo: repo, Preparer: m, Audit: dlq.ScopeAuditTrail{}}, nil
}

// dlqJobAudit records dlq.job_finished as the system job-runner actor (DL-203).
func dlqJobAudit(w *Wiring, m *dlq.Manager) jobs.AuditNotifier {
	actor := audit.Actor{Kind: audit.ActorSystem, Name: string(dlq.JobRunnerActor)}
	return jobs.AuditNotifier{Trail: dlq.RecorderAuditTrail{Recorder: w.Deps.Audit, Actor: actor}, Routes: m, Log: w.Deps.Log}
}

// dlqJobPublisher is the jobs.Publisher of DL-113 over the monitoring events bus.
func (w *Wiring) dlqJobPublisher() jobs.Publisher {
	pushFeature.build(w)
	return DLQJobBusPublisher{Out: pushFeature.publisher, Now: time.Now}
}

// DLQJobBusPublisher pushes dlq.job.* events workspace-wide (read is open across teams); the web client
// matches the job's requestedBy in the payload.
type DLQJobBusPublisher struct {
	Out events.Publisher
	Now func() time.Time
}

// Publish implements jobs.Publisher.
func (p DLQJobBusPublisher) Publish(ctx context.Context, eventType string, _ string, payload json.RawMessage) error {
	return p.Out.Publish(ctx, events.PushEvent{Type: events.PushEventType(eventType), At: p.Now().UTC(), Payload: payload})
}

// dlqPreview returns the preview service client used by Open in Studio.
func dlqPreview(w *Wiring) dlq.PreviewForwarder {
	settings, err := preview.NewSettingsLoader().Load(w.Deps.Config.Environment)
	if err != nil {
		return unavailablePreview{err: err}
	}
	return gateway.NewPreviewClient(settings.URL, nil, preview.NewDirectiveCodec(settings.DirectiveSecret, time.Now))
}

// dlqScripts starts the retry sandbox; a failure disables js steps (they then fail to compile).
func dlqScripts(ctx context.Context, w *Wiring, s dlq.Settings, salts host.SaltProvider) dlq.ScriptRuntimes {
	if s.Scripts == dlq.ScriptsNone {
		return dlq.NoScriptRuntimes{}
	}
	rt, err := dlq.NewSandboxScriptRuntimes(ctx, salts)
	if err != nil {
		w.Deps.Log.Error(string(dlq.LogDisabled), string(logging.AttrError), err)
		return dlq.NoScriptRuntimes{}
	}
	return rt
}

// unavailablePreview answers every preview call with the configuration error.
type unavailablePreview struct{ err error }

func (u unavailablePreview) Forward(context.Context, string, string, []byte, preview.Caller) (gateway.Upstream, error) {
	return gateway.Upstream{}, errors.Join(dlq.ErrStudioDown, u.err)
}
