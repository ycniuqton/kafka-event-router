package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/connections/pgstore"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/assembly"
	"github.com/eventrouter/router/internal/runtime/changes"
	"github.com/eventrouter/router/internal/runtime/clients"
	"github.com/eventrouter/router/internal/runtime/controlapi"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/scheduler"
	"github.com/eventrouter/router/internal/store/routestore"
	"github.com/eventrouter/router/internal/store/runtimestore"
	"github.com/eventrouter/router/pkg/kafka"
)

// SchedulerMetric names the leader gauge.
type SchedulerMetric string

const MetricSchedulerLeader SchedulerMetric = "router_scheduler_leader"

// runtimeWiring composes route-runtime inside the API once per process: internal worker API, public
// runtime/workers endpoints, the router_runtime change relay and the leader-only scheduler.
type runtimeWiring struct {
	once     sync.Once
	public   api.Module
	internal api.Module
	leader   *scheduler.LeaderState
	replays  *scheduler.ReplayService
}

var runtimeFeature = &runtimeWiring{leader: &scheduler.LeaderState{}}

// runtimeLeaderGate is the scheduler leadership for other features' leader-only jobs (monitoring alerts).
func (s *apiService) runtimeLeaderGate() scheduler.LeaderGate {
	return runtimeFeature.leader
}

// runtimeReplays is route-runtime's offset reset behind routes' OffsetResetter (POST /routes/{id}/replay);
// nil when route-runtime is disabled.
func (s *apiService) runtimeReplays() *scheduler.ReplayService {
	runtimeFeature.build(s)
	return runtimeFeature.replays
}

// runtimeModule returns GET /api/v1/routes/{id}/runtime and GET /api/v1/workers.
func (s *apiService) runtimeModule() api.Module {
	runtimeFeature.build(s)
	return runtimeFeature.public
}

// runtimeInternalModule returns the /internal/v1 worker endpoints.
func (s *apiService) runtimeInternalModule() api.Module {
	runtimeFeature.build(s)
	return runtimeFeature.internal
}

func (rw *runtimeWiring) build(s *apiService) {
	rw.once.Do(func() {
		w := s.wiring
		rw.public, rw.internal = api.Modules{}, api.Modules{}
		comps, err := w.RoutesComponents()
		if err != nil {
			w.Deps.Log.Error("route runtime disabled: routes unavailable", string(logging.AttrError), err)
			return
		}
		stores := runtimestore.New(w.Deps.DB, w.Tx)
		rw.replays = &scheduler.ReplayService{Stores: stores, Now: time.Now}
		timings := domain.DefaultTimings()
		lifecycle := routesLifecycle{c: comps}
		agg := &controlapi.RuntimeStatusAggregator{Assignments: stores.Assignments, Runtime: stores.Runtime}
		broker := changes.NewBroker()
		rw.internal = controlapi.NewInternalModule(controlapi.InternalDeps{
			Workers: &controlapi.WorkerAPI{Stores: stores, Aggregator: agg, Timings: timings, Now: time.Now},
			Halts:   &controlapi.HaltService{Stores: stores, Lifecycle: lifecycle, Pauser: routesPauser{c: comps, w: w}, Log: w.Deps.Log, Now: time.Now},
			Changes: broker, Responder: w.Responder,
		})
		rw.public = controlapi.NewPublicModule(controlapi.PublicDeps{Stores: stores, Responder: w.Responder, Pages: w.Pages, Timings: timings, Now: time.Now})
		rw.start(s, stores, agg, lifecycle, broker)
	})
}

// start runs the change relay and, when enabled, the scheduler until SIGINT/SIGTERM.
func (rw *runtimeWiring) start(s *apiService, stores state.Stores, agg *controlapi.RuntimeStatusAggregator, lc state.DeploymentLifecycle, broker *changes.Broker) {
	w := s.wiring
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go changes.PgListener{Notifications: w.Deps.DB.Notifications(), Broker: broker}.Run(ctx)
	loop, err := rw.scheduler(s, stores, agg, lc, broker)
	if err != nil || loop == nil {
		if err != nil {
			w.Deps.Log.Error("scheduler disabled", string(logging.AttrError), err)
		}
		return
	}
	go func() {
		defer stop()
		loop.Run(ctx)
	}()
}

// scheduler builds the leader-only loop (nil when ROUTER_SCHEDULER_ENABLED=false).
func (rw *runtimeWiring) scheduler(s *apiService, stores state.Stores, agg *controlapi.RuntimeStatusAggregator, lc state.DeploymentLifecycle, broker *changes.Broker) (*scheduler.SchedulerLoop, error) {
	cfg, err := config.NewRuntimeLoader().Scheduler()
	if err != nil || !cfg.Enabled {
		return nil, err
	}
	placement, err := scheduler.NewDefaultPlacementManager(domain.PlacementKind(cfg.Placement))
	if err != nil {
		return nil, err
	}
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: string(MetricSchedulerLeader)})
	s.registry.Registerer().MustRegister(gauge)
	timings := domain.DefaultTimings()
	return &scheduler.SchedulerLoop{
		Elector: scheduler.PgLeaderElector{Pool: s.wiring.Deps.DB.Pool(), Key: scheduler.SchedulerLockKey, Gauge: gauge},
		Reaper:  scheduler.LeaseReaper{Leases: stores.Assignments, Now: time.Now},
		Planner: &scheduler.AssignmentPlanner{Stores: stores, Placement: placement, Runtime: agg, Timings: timings, Now: time.Now},
		Rollout: &scheduler.RolloutCoordinator{Stores: stores, Lifecycle: lc, Timings: timings, Now: time.Now},
		Replay:  scheduler.NewReplayCoordinator(stores, newReplayClusters(s.wiring), timings, s.wiring.Deps.Log, time.Now),
		Hints:   broker, Timings: timings, Log: s.wiring.Deps.Log, State: rw.leader,
	}, nil
}

// routesLifecycle adapts routes' DeploymentLifecycle (MarkFailed creates the auto_rollback version).
type routesLifecycle struct {
	c *assembly.Components
}

func (l routesLifecycle) MarkActive(ctx context.Context, id string) error {
	_, err := l.c.RolloutReporter().MarkActive(ctx, routes.DeploymentID(id))
	return err
}

func (l routesLifecycle) MarkFailed(ctx context.Context, id, reason string) (bool, error) {
	out, err := l.c.RolloutReporter().MarkFailed(ctx, routes.DeploymentID(id), reason)
	return out.Paused, err
}

// routesPauser pauses a halted route through the routes repository and announces it on route_changed.
type routesPauser struct {
	c *assembly.Components
	w *Wiring
}

func (p routesPauser) Pause(ctx context.Context, id string) error {
	route, err := p.c.Repos.Routes.SetDesiredState(ctx, routes.RouteID(id), routes.DesiredPaused, nil)
	if err != nil {
		return err
	}
	return routestore.PgNotifyPublisher{Notifier: p.w.Notifier}.Publish(ctx, routes.ChangeOf(route, routes.ChangePaused))
}

// replayClusters opens admin clients on saved connections (secrets decrypted in the API) for replays.
type replayClusters struct {
	specs  *connections.SpecResolver
	opener *clients.OpenerManager
}

func newReplayClusters(w *Wiring) replayClusters {
	repo := pgstore.NewConnectionRepository(w.Deps.DB, w.Tx)
	return replayClusters{specs: connections.NewSpecResolver(repo, connections.NewSecretVault(w.Deps.Secrets)),
		opener: clients.NewOpenerManager(clients.NewFranzOpener(), kafka.DefaultDemoClientFactory())}
}

// Admin implements scheduler.ReplayClusters.
func (c replayClusters) Admin(ctx context.Context, connectionID string) (kafka.Client, error) {
	id, err := uuid.Parse(connectionID)
	if err != nil {
		return nil, err
	}
	_, spec, err := c.specs.Resolve(ctx, id)
	if err != nil {
		return nil, err
	}
	return c.opener.Admin(ctx, spec)
}
