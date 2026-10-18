package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/connections/checks"
	connhttp "github.com/eventrouter/router/internal/connections/http"
	"github.com/eventrouter/router/internal/connections/pgstore"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/routes/assembly"
	"github.com/eventrouter/router/pkg/kafka"
)

// connectionsWiring composes the connections feature once per process and starts its health prober.
type connectionsWiring struct {
	once     sync.Once
	public   api.Module
	internal api.Module
	service  *connections.Service
	topics   *connections.TopicBrowser
}

var connectionsFeature = &connectionsWiring{}

// connectionsModule returns the public /api/v1/connections module.
func (s *apiService) connectionsModule() api.Module {
	connectionsFeature.build(s.wiring)
	return connectionsFeature.public
}

// connectionsService returns the connections entry point for other features (e.g. monitoring's demo seeder
// calling CreateDemo / DeleteDemo).
func (s *apiService) connectionsService() *connections.Service {
	connectionsFeature.build(s.wiring)
	return connectionsFeature.service
}

// connectionsInternalModule returns the workload-only /internal/v1/connections module.
func (s *apiService) connectionsInternalModule() api.Module {
	connectionsFeature.build(s.wiring)
	return connectionsFeature.internal
}

func (c *connectionsWiring) build(w *Wiring) {
	c.once.Do(func() {
		deps := w.Deps
		settings, err := connections.SettingsLoader{Lookup: os.LookupEnv}.Load()
		if err != nil {
			deps.Log.Error("connections settings invalid; using defaults", string(logging.AttrError), err)
		}
		security, props := kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog()
		clients := kafka.NewClientFactoryRegistry()
		clients.Register(kafka.NewFranzClientFactory(security, props, kafka.DefaultPurposeDefaults()))
		demo := kafka.DefaultDemoClientFactory()
		clients.Register(demo)
		repo := pgstore.NewConnectionRepository(deps.DB, w.Tx)
		vault := connections.NewSecretVault(deps.Secrets)
		validator := connections.DefaultValidator(security, props)
		resolver := connections.NewSpecResolver(repo, vault)
		failures := connections.ClusterFailures{Classifier: kafka.DefaultFailureClassifier(), Hints: connections.DefaultHintCatalog()}
		runner := connections.NewCheckRunner(checks.DefaultRegistry(), failures.Classifier, failures.Hints, nil)
		if err := runner.Validate(settings.TestPlan); err != nil {
			deps.Log.Error("connections test plan invalid; using default", string(logging.AttrError), err)
			settings.TestPlan = connections.DefaultTestPlan
		}
		authz, err := w.Authorizers.Build(deps.Config.Auth.Authz)
		if err != nil {
			deps.Log.Error("connections authorizer unavailable; team guard off", string(logging.AttrError), err)
		}
		c.service = connections.NewService(connections.ServiceDeps{Repo: repo, Usage: pgstore.NewUsageResolver(deps.DB), Vault: vault,
			Validator: validator, Tx: w.Tx, Access: c.access(w), Teardown: connections.DemoClusterTeardown{Pool: demo.Pool()}})
		c.topics = connections.NewTopicBrowser(resolver, clients, connections.NewRedisRateEstimator(deps.Redis, nil), failures)
		d := connhttp.Deps{
			Service:  c.service,
			Authz:    authz,
			Tester:   connections.NewTester(connections.TesterDeps{Repo: repo, Resolver: resolver, Vault: vault, Validator: validator, Clients: clients, Runner: runner, Plan: settings.TestPlan}),
			Topics:   c.topics,
			Creator:  connections.NewTopicCreator(resolver, clients, failures),
			Resolver: resolver, Responder: w.Responder, Pages: w.Pages, Recorder: deps.Audit,
		}
		c.public, c.internal = connhttp.NewModule(d), connhttp.NewInternalModule(d)
		c.startProber(w, settings, repo, vault, clients, runner)
	})
}

// access returns access-control's grant check (owner-only fallback when access control is not wired).
func (c *connectionsWiring) access(w *Wiring) connections.ConnectionAccess {
	if w.Access == nil || w.Access.Manager == nil {
		return connections.OwnerOnlyAccess{}
	}
	return w.Access.Manager
}

// startProber runs the 60 s health probe until SIGINT/SIGTERM.
func (c *connectionsWiring) startProber(w *Wiring, s connections.Settings, repo connections.Repository, vault *connections.SecretVault, clients connections.ClientOpener, runner *connections.CheckRunner) {
	if !s.ProbeEnabled {
		return
	}
	prober := connections.NewHealthProber(connections.ProberDeps{
		Repo: repo, Vault: vault, Clients: clients, Runner: runner, Leader: connections.AlwaysLeader{},
		Publisher: w.connectionHealthPublisher(), Log: w.Deps.Log,
	}, connections.ProberConfig{Interval: s.ProbeInterval, Workers: int(connections.DefaultProbeWorkers), Batch: int(connections.DefaultProbeBatch), Plan: connections.DefaultProbePlan})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		defer stop()
		prober.Run(ctx)
	}()
}

func init() {
	routesPortContributors = append(routesPortContributors, contributeTopicInspector)
}

// contributeTopicInspector gives routes the destination-topic check (SysDesign §5: the destination topic exists
// or createTopic is set) through the connections topic browser.
func contributeTopicInspector(_ *Wiring, p *assembly.Ports) {
	p.Topics = routesTopicInspector{}
}

// ErrTopicsUnavailable is returned while the connections feature is not built yet (the check becomes a warning).
var ErrTopicsUnavailable = errors.New("connections: topic browser not available")

// routesTopicInspector reads the connections topic browser at call time (connections is built after routes).
type routesTopicInspector struct{}

// TopicExists implements validation.TopicInspector.
func (routesTopicInspector) TopicExists(ctx context.Context, connectionID, topic string) (bool, error) {
	browser := connectionsFeature.topics
	if browser == nil {
		return false, ErrTopicsUnavailable
	}
	return browser.TopicExists(ctx, connectionID, topic)
}
