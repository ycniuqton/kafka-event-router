package metricshttp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/connections/connectionstest"
	"github.com/eventrouter/router/internal/connections/pgstore"
	"github.com/eventrouter/router/internal/metrics/dashboard"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/testkit"
	"github.com/eventrouter/router/pkg/kafka"
)

// demoEnv is the demo endpoints over a real connections service on a throwaway Postgres schema.
type demoEnv struct {
	handler http.Handler
	repo    *pgstore.ConnectionRepository
	team    uuid.UUID
}

func newDemoEnv(t *testing.T) demoEnv {
	t.Helper()
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	var team uuid.UUID
	if err := sb.DB.Pool().QueryRow(context.Background(), `INSERT INTO teams (name) VALUES ('demo-p3') RETURNING id`).Scan(&team); err != nil {
		t.Fatal(err)
	}
	tx := store.NewTxManager(sb.DB)
	repo := pgstore.NewConnectionRepository(sb.DB, tx)
	svc := connections.NewService(connections.ServiceDeps{Repo: repo, Usage: pgstore.NewUsageResolver(sb.DB), Tx: tx,
		Vault:     connections.NewSecretVault(connectionstest.NewMemSecrets()),
		Validator: connections.DefaultValidator(kafka.DefaultSecurityRegistry(), kafka.DefaultPropertyCatalog())})
	responder := api.NewResponder(problem.NewCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	mod := NewModule(Deps{Onboarding: dashboard.OnboardingService{Counts: dashboard.NewPgDashboardStore(sb.DB, pgstore.NewUsageResolver(sb.DB))},
		Demo: dashboard.DemoWorkspaceSeeder{Connections: svc}, Responder: responder})
	r := chi.NewRouter()
	p := auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New(), Teams: []auth.TeamRole{{TeamID: team, Role: auth.RoleEditor}}}
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(auth.WithPrincipal(req.Context(), p)))
		})
	})
	api.NewMounter(responder, nil, nil).Mount(r, mod)
	return demoEnv{handler: r, repo: repo, team: team}
}

func (e demoEnv) call(method string, path Path) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, httptest.NewRequest(method, string(path), nil))
	return rec
}

func (e demoEnv) onboarding(t *testing.T) dashboard.Onboarding {
	t.Helper()
	rec := e.call(http.MethodGet, PathOnboarding)
	var o dashboard.Onboarding
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &o) != nil {
		t.Fatalf("onboarding %d %s", rec.Code, rec.Body)
	}
	return o
}

func TestIntegrationDemoSeedIsIdempotentProducesAndCleansUp(t *testing.T) {
	e := newDemoEnv(t)
	if o := e.onboarding(t); !o.DemoAvailable || o.Demo != nil {
		t.Fatalf("before seed %+v", o)
	}
	rec := e.call(http.MethodPost, PathDemo)
	var seed dashboard.DemoSeed
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &seed) != nil || seed.Topic != string(kafka.DemoOrderEvents) {
		t.Fatalf("create %d %s", rec.Code, rec.Body)
	}
	t.Cleanup(func() { kafka.SharedDemoClusterPool().Release(kafka.DemoClusterKey(seed.ConnectionID)) })
	if again := e.call(http.MethodPost, PathDemo); again.Code != http.StatusConflict {
		t.Fatalf("second create %d %s", again.Code, again.Body)
	}
	if o := e.onboarding(t); o.Demo == nil || o.Demo.ConnectionID != seed.ConnectionID {
		t.Fatalf("after seed %+v", o)
	}
	e.assertGeneratorProduces(t, seed)
	if del := e.call(http.MethodDelete, PathDemo); del.Code != http.StatusNoContent {
		t.Fatalf("delete %d %s", del.Code, del.Body)
	}
	if o := e.onboarding(t); o.Demo != nil {
		t.Fatalf("after delete %+v", o)
	}
	if del := e.call(http.MethodDelete, PathDemo); del.Code != http.StatusNoContent {
		t.Fatalf("second delete %d", del.Code)
	}
}

// assertGeneratorProduces checks that order-events grows on the seeded demo cluster.
func (e demoEnv) assertGeneratorProduces(t *testing.T, seed dashboard.DemoSeed) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c, err := e.repo.Get(ctx, uuid.MustParse(seed.ConnectionID))
	if err != nil || c.Kind != kafka.ClusterDemo {
		t.Fatalf("stored %+v %v", c, err)
	}
	cl, err := kafka.DefaultClientFactoryRegistry().NewClient(ctx, c.Spec(kafka.SecretBundle{}), kafka.PurposeAdmin)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	topic := string(kafka.DemoOrderEvents)
	first, _ := cl.Admin().EndOffsets(ctx, topic)
	for ctx.Err() == nil {
		now, _ := cl.Admin().EndOffsets(ctx, topic)
		if now[topic] > first[topic] && first[topic] > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("demo generator produced nothing to order-events")
}
