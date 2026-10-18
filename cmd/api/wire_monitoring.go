package main

import (
	"os"
	"sync"
	"time"

	alertspg "github.com/eventrouter/router/internal/alerts/pgstore"
	"github.com/eventrouter/router/internal/connections/pgstore"
	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/metrics/dashboard"
	"github.com/eventrouter/router/internal/metrics/flowmap"
	metricshttp "github.com/eventrouter/router/internal/metrics/http"
	"github.com/eventrouter/router/internal/metrics/overview"
	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/metrics/routelist"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/routes/assembly"
)

// MonitoringLog is the log message of a disabled monitoring module.
type MonitoringLog string

const LogMonitoringDisabled MonitoringLog = "monitoring module disabled"

// RoutesPortContributor adds another folder's collaborator to the routes assembly ports before it is built.
type RoutesPortContributor func(w *Wiring, ports *assembly.Ports)

// routesPortContributors are applied by routesPorts in registration order (folders append from init).
var routesPortContributors []RoutesPortContributor

// routesPorts returns the routes ports gathered from every contributor.
func (w *Wiring) routesPorts() assembly.Ports {
	var ports assembly.Ports
	for _, c := range routesPortContributors {
		c(w, &ports)
	}
	return ports
}

func init() {
	routesPortContributors = append(routesPortContributors, monitoringFeature.contributeRouteMetrics)
}

// monitoringWiring composes the monitoring feature once per process.
type monitoringWiring struct {
	once     sync.Once
	settings metrics.Settings
	querier  query.Querier
	err      error
}

var monitoringFeature = &monitoringWiring{}

// core loads settings and selects the metrics backend (once).
func (m *monitoringWiring) core(w *Wiring) error {
	m.once.Do(func() {
		m.settings, m.err = metrics.SettingsLoader{Lookup: os.LookupEnv}.Load()
		if m.err != nil {
			return
		}
		m.querier, m.err = query.NewQuerierManager().Select(m.settings.Backend,
			query.BackendConfig{URL: m.settings.URL, Timeout: m.settings.Timeout})
	})
	return m.err
}

// contributeRouteMetrics registers PromRouteMetricsReader for ROUTER_ROUTE_METRICS_READER=prometheus.
func (m *monitoringWiring) contributeRouteMetrics(w *Wiring, ports *assembly.Ports) {
	if err := m.core(w); err != nil {
		return
	}
	ports.MetricsReaders = append(ports.MetricsReaders, &routelist.PromRouteMetricsReader{Querier: m.querier, Now: time.Now})
}

// monitoringModule returns the dashboard, activity, onboarding, route metrics and flow-map endpoints plus
// alert rules, the bell feed and WS /events (P2).
func (s *apiService) monitoringModule() api.Module {
	core := s.monitoringCoreModule()
	hs := append(append([]api.Handler{}, core.Handlers()...), s.alertsModule().Handlers()...)
	return api.Modules(append(hs, s.liveTailModule().Handlers()...))
}

// monitoringCoreModule returns the MVP dashboard endpoints and the flow map.
func (s *apiService) monitoringCoreModule() api.Module {
	w, m := s.wiring, monitoringFeature
	if err := m.core(w); err != nil {
		w.Deps.Log.Error(string(LogMonitoringDisabled), string(logging.AttrError), err)
		return api.Modules{}
	}
	c, err := w.RoutesComponents()
	if err != nil {
		w.Deps.Log.Error(string(LogMonitoringDisabled), string(logging.AttrError), err)
		return api.Modules{}
	}
	pg := dashboard.NewPgDashboardStore(w.Deps.DB, pgstore.NewUsageResolver(w.Deps.DB))
	t := m.settings.Thresholds
	attention, err := dashboard.NewAttentionManager(pg, t, w.Deps.Log, m.settings.Detectors, dashboard.RouteErrorDetector{},
		dashboard.ConnectionHealthDetector{Connections: pg}, dashboard.LagGrowthDetector{Querier: m.querier},
		dashboard.ZeroMatchDetector{Querier: m.querier}, dashboard.FiringAlertDetector{Alerts: alertspg.NewEventStore(w.Deps.DB)})
	if err != nil {
		w.Deps.Log.Error(string(LogMonitoringDisabled), string(logging.AttrError), err)
		return api.Modules{}
	}
	return metricshttp.NewModule(metricshttp.Deps{
		Summary: &dashboard.SummaryService{Counter: dashboard.RouteStateCounter{Routes: pg}, Querier: m.querier, Thresholds: t,
			Now: time.Now, Log: w.Deps.Log},
		Attention: attention, Activity: dashboard.ActivityService{Audit: w.Access.Audit},
		Onboarding: dashboard.OnboardingService{Counts: pg},
		Overview:   &overview.RouteOverviewService{Routes: c.Repos.Routes, Querier: m.querier, Now: time.Now},
		FlowMap:    &flowmap.FlowMapService{Store: flowmap.PgStore{DB: w.Deps.DB}, Querier: m.querier, Log: w.Deps.Log},
		Demo:       s.demoSeeder(),
		Responder:  w.Responder, Pages: w.Pages,
	})
}

// demoSeeder is the "Try with demo data" seeder over connections' demo service (nil when connections is off).
func (s *apiService) demoSeeder() metricshttp.DemoSeeder {
	svc := s.connectionsService()
	if svc == nil {
		return nil
	}
	return dashboard.DemoWorkspaceSeeder{Connections: svc}
}
