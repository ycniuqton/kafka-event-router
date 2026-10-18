package runtimestore_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/store/routestore"
	"github.com/eventrouter/router/internal/store/runtimestore"
	"github.com/eventrouter/router/internal/testkit"
)

// replaySandbox is a migrated schema holding one running route with an active version.
func replaySandbox(t *testing.T) (state.Stores, string) {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	ctx := context.Background()
	tx := store.NewTxManager(sb.DB)
	var team string
	if err := sb.DB.Pool().QueryRow(ctx, `INSERT INTO teams (name) VALUES ('payments') RETURNING id::text`).Scan(&team); err != nil {
		t.Fatal(err)
	}
	r, err := routestore.NewRouteRepository(sb.DB).Create(ctx, routes.Route{TeamID: routes.TeamID(team), Name: "orders", DesiredState: routes.DesiredRunning,
		Replicas: 1, Draft: json.RawMessage(`{"name":"orders"}`)})
	if err != nil {
		t.Fatal(err)
	}
	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		v, err := routestore.NewVersionRepository(sb.DB).Create(ctx, routes.RouteVersion{RouteID: r.ID, Config: json.RawMessage(`{}`), ConfigHash: "h", Origin: routes.OriginDeploy})
		if err != nil {
			return err
		}
		_, err = sb.DB.Q(ctx).Exec(ctx, `UPDATE routes SET active_version_id = $2::uuid WHERE id = $1::uuid`, string(r.ID), string(v.ID))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtimestore.New(sb.DB, tx), string(r.ID)
}

// TestReplayRepositoryLifecycle covers create, one-open-per-route, transitions and the planner's Replaying flag.
func TestReplayRepositoryLifecycle(t *testing.T) {
	stores, routeID := replaySandbox(t)
	ctx := context.Background()
	from := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	rp, err := stores.Replays.Create(ctx, state.Replay{RouteID: routeID, From: from, RequestedBy: "not-a-uuid"})
	if err != nil || rp.Status != domain.ReplayPending || !rp.From.Equal(from) || rp.RequestedBy != "" {
		t.Fatalf("create: %+v %v", rp, err)
	}
	if _, err := stores.Replays.Create(ctx, state.Replay{RouteID: routeID, From: from}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("second open replay must conflict: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	moved, err := stores.Replays.Transition(ctx, state.ReplayTransition{ID: rp.ID, From: domain.ReplayPending, To: domain.ReplayStopping, At: now})
	if err != nil || !moved {
		t.Fatalf("pending -> stopping: %v %v", moved, err)
	}
	if again, _ := stores.Replays.Transition(ctx, state.ReplayTransition{ID: rp.ID, From: domain.ReplayPending, To: domain.ReplayStopping, At: now}); again {
		t.Fatal("transition must be compare-and-set")
	}
	demands, err := stores.Routes.Demands(ctx)
	if err != nil || len(demands) != 1 || !demands[0].Replaying || demands[0].Schedulable() {
		t.Fatalf("stopping replay must suspend the route: %+v %v", demands, err)
	}
	steps := []state.ReplayTransition{
		{ID: rp.ID, From: domain.ReplayStopping, To: domain.ReplayResetting, At: now, Group: "router.g"},
		{ID: rp.ID, From: domain.ReplayResetting, To: domain.ReplayDone, At: now, Partitions: 3},
	}
	for _, s := range steps {
		if ok, err := stores.Replays.Transition(ctx, s); err != nil || !ok {
			t.Fatalf("%s -> %s: %v %v", s.From, s.To, ok, err)
		}
	}
	done, err := stores.Replays.Get(ctx, routeID, rp.ID)
	if err != nil || done.Status != domain.ReplayDone || done.ConsumerGroup != "router.g" || done.Partitions != 3 || done.StartedAt == nil || done.FinishedAt == nil {
		t.Fatalf("done replay: %+v %v", done, err)
	}
	if open, err := stores.Replays.Open(ctx); err != nil || len(open) != 0 {
		t.Fatalf("no replay may stay open: %+v %v", open, err)
	}
	if _, err := stores.Replays.Create(ctx, state.Replay{RouteID: routeID, From: from}); err != nil {
		t.Fatalf("a finished replay must not block the next one: %v", err)
	}
}
