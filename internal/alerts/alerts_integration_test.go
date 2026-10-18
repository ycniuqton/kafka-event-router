package alerts_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/alerts/alertstest"
	"github.com/eventrouter/router/internal/alerts/engine"
	"github.com/eventrouter/router/internal/alerts/evaluator"
	"github.com/eventrouter/router/internal/alerts/notify"
	"github.com/eventrouter/router/internal/alerts/pgstore"
	"github.com/eventrouter/router/internal/events"
	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/testkit"
)

// fakeProm answers every instant query with the current lag value.
type fakeProm struct{ lag atomic.Int64 }

func (p *fakeProm) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	body := fmt.Sprintf(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[%d,"%d"]}]}}`,
		time.Now().Unix(), p.lag.Load())
	_, _ = w.Write([]byte(body))
}

type hookLog struct {
	mu    sync.Mutex
	types []string
}

func (h *hookLog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var p notify.WebhookPayload
	_ = json.NewDecoder(r.Body).Decode(&p)
	h.mu.Lock()
	h.types = append(h.types, string(p.Type))
	h.mu.Unlock()
}

type oneSecret struct{ url string }

func (s oneSecret) Get(context.Context, secrets.Ref) ([]byte, error) { return []byte(s.url), nil }

type leader struct{}

func (leader) IsLeader() bool { return true }

type busOut struct{ n atomic.Int32 }

func (b *busOut) Publish(context.Context, events.PushEvent) error { b.n.Add(1); return nil }

// TestLagRuleFiresNotifiesOnceAndResolves (P2-T-04) runs the evaluator against Postgres (testkit sandbox) and an
// HTTP Prometheus: lag above N fires once, the webhook is notified once, and the alert resolves after a clean window.
func TestLagRuleFiresNotifiesOnceAndResolves(t *testing.T) {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	ctx := context.Background()
	var team, route uuid.UUID
	pool := sb.DB.Pool()
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('it') RETURNING id`).Scan(&team); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO routes (team_id, name, desired_state) VALUES ($1, 'orders', 'running') RETURNING id`, team).Scan(&route); err != nil {
		t.Fatal(err)
	}
	prom, hooks := &fakeProm{}, &hookLog{}
	promSrv, hookSrv := httptest.NewServer(prom), httptest.NewServer(hooks)
	defer promSrv.Close()
	defer hookSrv.Close()
	querier, err := query.NewPrometheusQuerier(query.BackendConfig{URL: promSrv.URL, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	rules, evs, routes := pgstore.NewRuleStore(sb.DB), pgstore.NewEventStore(sb.DB), pgstore.NewRouteDirectory(sb.DB)
	rule, err := rules.Insert(ctx, alerts.AlertRule{RouteID: route, Name: "lag", Kind: alerts.KindLagAbove, Threshold: 1000, Window: 1,
		Enabled: true, Channels: []alerts.ChannelRef{{Type: alerts.ChannelWebhook, SecretRef: "ref"}}})
	if err != nil {
		t.Fatal(err)
	}
	clock := alertstest.NewClock(time.Now())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	out := &busOut{}
	job := &engine.AlertEvaluatorJob{Leader: leader{}, Rules: rules, Routes: routes, Evaluators: evaluator.NewDefaultEvaluatorManager(),
		Querier: querier, Tracker: &engine.StateTracker{Tx: store.NewTxManager(sb.DB), Events: evs, Out: out,
			Factory: events.EventFactory{Now: clock.Now}, Now: clock.Now},
		Dispatcher: &notify.NotificationDispatcher{Notifiers: notify.NewNotifierManager(nil, notify.WebhookNotifier{
			Poster: notify.JSONPoster{Client: hookSrv.Client()}, Now: clock.Now}), Secrets: oneSecret{url: hookSrv.URL}, Events: evs,
			Rules: rules, Routes: routes, Counter: notify.MetricsCounter{Metrics: alerts.NewMetrics(nil)}, MaxAttempts: 3, Now: clock.Now, Log: log},
		Errors: alerts.NewMetrics(nil), Interval: time.Second, Log: log}
	tick := func(lag int64) {
		prom.lag.Store(lag)
		clock.Advance(30 * time.Second)
		if err := job.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	tick(10)
	tick(5000)
	tick(6000)
	tick(7000)
	firing, err := evs.Firing(ctx, rule.ID)
	if err != nil || firing == nil || *firing.Value != 5000 || firing.NotifiedAt == nil {
		t.Fatalf("firing: %+v %v", firing, err)
	}
	tick(10)
	tick(10)
	tick(10)
	if f, _ := evs.Firing(ctx, rule.ID); f != nil {
		t.Fatal("must resolve after a clean window")
	}
	hooks.mu.Lock()
	defer hooks.mu.Unlock()
	if fmt.Sprint(hooks.types) != "[alert.fired alert.resolved]" || out.n.Load() != 2 {
		t.Fatalf("webhooks=%v push=%d", hooks.types, out.n.Load())
	}
	page, err := evs.Feed(ctx, alerts.FeedQuery{User: uuid.New(), Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].State != alerts.StateResolved || page.Items[0].RouteName != "orders" {
		t.Fatalf("feed: %+v %v", page, err)
	}
}
