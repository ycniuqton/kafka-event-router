package main

import (
	"context"
	"net/http"
	"net/smtp"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/alerts/engine"
	"github.com/eventrouter/router/internal/alerts/evaluator"
	alertshttp "github.com/eventrouter/router/internal/alerts/http"
	"github.com/eventrouter/router/internal/alerts/notify"
	"github.com/eventrouter/router/internal/alerts/pgstore"
	"github.com/eventrouter/router/internal/alerts/service"
	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/events"
	eventshttp "github.com/eventrouter/router/internal/events/http"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/runtime/changes"
)

// pushWiring composes WS /events once per process: bus, hub, publishers and the runtime change source.
type pushWiring struct {
	once      sync.Once
	settings  events.Settings
	hub       *events.Hub
	publisher events.Publisher
	module    api.Module
}

var pushFeature = &pushWiring{}

// build selects the bus and visibility, starts the hub relay and the runtime change source.
func (p *pushWiring) build(w *Wiring) {
	p.once.Do(func() {
		log := w.Deps.Log
		p.settings = events.SettingsLoader{Lookup: os.LookupEnv}.Load()
		vis, err := events.NewVisibilityManager().Select(p.settings.Visibility)
		if err != nil {
			log.Error(string(LogMonitoringDisabled), string(logging.AttrError), err)
			vis = events.OpenVisibility{}
		}
		p.hub = events.NewHub(vis, log)
		bus, err := events.NewBusManager(w.Deps.Redis, log).Select(p.settings.Bus)
		if err != nil {
			log.Error(string(LogMonitoringDisabled), string(logging.AttrError), err)
			bus = events.NewMemoryBus()
		}
		p.publisher = events.FallbackPublisher{Bus: bus, Local: p.hub, Log: log}
		p.module = w.WebSockets.Modules(&eventshttp.EventsHandler{Hub: p.hub, Visibility: vis.Kind(), Now: time.Now})
		ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		go p.hub.Run(ctx, bus)
		p.startRuntimeSource(ctx, w)
	})
}

// startRuntimeSource LISTENs on router_runtime (own connection) and pushes route/deployment changes locally.
func (p *pushWiring) startRuntimeSource(ctx context.Context, w *Wiring) {
	broker := changes.NewBroker()
	go changes.PgListener{Notifications: w.Deps.DB.Notifications(), Broker: broker}.Run(ctx)
	src := &events.RuntimeChangeSource{Hints: broker, Reader: events.PgRouteStateReader{DB: w.Deps.DB}, Out: p.hub,
		Factory: events.EventFactory{Now: time.Now}, Log: w.Deps.Log}
	go src.Run(ctx)
}

// connectionHealthPublisher is connections' HealthEventPublisher (connection.health_changed on the bus).
func (w *Wiring) connectionHealthPublisher() connections.HealthEventPublisher {
	pushFeature.build(w)
	return events.ConnectionHealthPublisher{Out: pushFeature.publisher, Factory: events.EventFactory{Now: time.Now}}
}

// alertsWiring composes alert rules, the bell feed and the leader-only evaluator once per process.
type alertsWiring struct {
	once   sync.Once
	module api.Module
	feed   *service.FeedService
}

var alertsFeature = &alertsWiring{}

// build wires stores, notifiers, services and starts the evaluator job.
func (a *alertsWiring) build(s *apiService) {
	a.once.Do(func() {
		w := s.wiring
		settings, err := alerts.SettingsLoader{Lookup: os.LookupEnv}.Load()
		if err != nil {
			w.Deps.Log.Error(string(LogMonitoringDisabled), string(logging.AttrError), err)
			a.module = api.Modules{}
			return
		}
		pushFeature.build(w)
		rules, evs, routes := pgstore.NewRuleStore(w.Deps.DB), pgstore.NewEventStore(w.Deps.DB), pgstore.NewRouteDirectory(w.Deps.DB)
		m := alerts.NewMetrics(s.registry.Registerer())
		links := notify.LinkBuilder{PublicURL: w.Deps.Config.HTTP.PublicURL}
		dispatcher := &notify.NotificationDispatcher{Notifiers: a.notifiers(settings), Secrets: w.Deps.Secrets, Events: evs,
			Rules: rules, Routes: routes, Links: links, Counter: notify.MetricsCounter{Metrics: m}, MaxAttempts: settings.MaxAttempts,
			Now: time.Now, Log: w.Deps.Log}
		a.feed = &service.FeedService{Events: evs, Now: time.Now}
		a.module = alertshttp.NewModule(alertshttp.Deps{Rules: &service.RuleService{Rules: rules, Routes: routes,
			Secrets: service.ChannelSecretStore{Secrets: w.Deps.Secrets}, Access: alertAccess{w: w}, Tx: w.Tx, Deliver: dispatcher,
			Links: links}, Feed: a.feed, Responder: w.Responder, Pages: w.Pages})
		if settings.Enabled {
			a.start(s, settings, rules, evs, routes, dispatcher, m)
		}
	})
}

// notifiers registers every channel implementation; the enabled set comes from ROUTER_ALERTS_CHANNELS.
func (a *alertsWiring) notifiers(s alerts.Settings) *notify.NotifierManager {
	poster := notify.JSONPoster{Client: &http.Client{Timeout: s.HTTPTimeout}}
	smtpCfg := notify.SMTPSettings{Host: s.SMTP.Host, Port: s.SMTP.Port, User: s.SMTP.User, Password: s.SMTP.Password, From: s.SMTP.From}
	return notify.NewNotifierManager(s.Channels,
		notify.WebhookNotifier{Poster: poster, SigningKey: []byte(s.SigningKey), Now: time.Now},
		notify.SlackNotifier{Poster: poster}, notify.TeamsNotifier{Poster: poster},
		notify.EmailNotifier{SMTP: smtpCfg, Mailer: smtp.SendMail})
}

// start runs the evaluator every interval on the scheduler leader until SIGINT/SIGTERM.
func (a *alertsWiring) start(s *apiService, st alerts.Settings, rules *pgstore.RuleStore, evs *pgstore.EventStore,
	routes *pgstore.RouteDirectory, d *notify.NotificationDispatcher, m *alerts.Metrics) {
	w := s.wiring
	if err := monitoringFeature.core(w); err != nil {
		w.Deps.Log.Error(string(LogMonitoringDisabled), string(logging.AttrError), err)
		return
	}
	job := &engine.AlertEvaluatorJob{Leader: s.runtimeLeaderGate(), Rules: rules, Routes: routes,
		Evaluators: evaluator.NewDefaultEvaluatorManager(), Querier: monitoringFeature.querier,
		Tracker:    &engine.StateTracker{Tx: w.Tx, Events: evs, Out: pushFeature.publisher, Factory: events.EventFactory{Now: time.Now}, Now: time.Now},
		Dispatcher: d, Errors: m, Interval: st.Interval, Log: w.Deps.Log, Defaults: alertDefaults(w)}
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go job.Run(ctx)
}

// alertsModule returns /alert-rules, /alerts and WS /events.
func (s *apiService) alertsModule() api.Module {
	alertsFeature.build(s)
	pushFeature.build(s.wiring)
	return api.Modules(append(append([]api.Handler{}, alertsFeature.module.Handlers()...), pushFeature.module.Handlers()...))
}

// alertAccess checks alert.write on the route's team through access control.
type alertAccess struct {
	w *Wiring
}

// CanWriteAlerts implements service.RuleAccess.
func (a alertAccess) CanWriteAlerts(ctx context.Context, team uuid.UUID) bool {
	if a.w.Access == nil || a.w.Access.Manager == nil {
		return true
	}
	return a.w.Access.Manager.Decide(ctx, rbac.PermAlertWrite, rbac.Resource{Type: rbac.ResourceRoute, TeamID: team}).Allowed
}
