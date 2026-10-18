// Stack test of a replay on the running local stack (RR-208): ReplayService records the request in the real
// route_replays table and the live API scheduler (leader) carries it out. Opt-in: set ROUTER_IT_DATABASE_URL,
// ROUTER_IT_REPLAY_ROUTE (a deployed route id) and ROUTER_IT_REPLAY_FROM (RFC 3339).
package scheduler_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/scheduler"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/store/runtimestore"
)

// replayStack holds the env knobs and the time the live scheduler gets to finish a replay.
var replayStack = struct {
	EnvRoute, EnvFrom string
	Finish, Poll      time.Duration
}{EnvRoute: "ROUTER_IT_REPLAY_ROUTE", EnvFrom: "ROUTER_IT_REPLAY_FROM", Finish: 2 * time.Minute, Poll: time.Second}

func TestReplayOnLiveStack(t *testing.T) {
	dsn, routeID := os.Getenv(leaderIT.EnvDSN), os.Getenv(replayStack.EnvRoute)
	from, err := time.Parse(time.RFC3339Nano, os.Getenv(replayStack.EnvFrom))
	if dsn == "" || routeID == "" || err != nil {
		t.Skip("set ROUTER_IT_DATABASE_URL, ROUTER_IT_REPLAY_ROUTE and ROUTER_IT_REPLAY_FROM to replay on the local stack")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{URL: dsn})
	if err != nil {
		t.Fatal(err)
	}
	svc := &scheduler.ReplayService{Stores: runtimestore.New(db, store.NewTxManager(db)), Now: time.Now}
	rp, err := svc.Reset(ctx, scheduler.ReplayRequest{RouteID: routeID, From: from})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[domain.ReplayStatus]bool{}
	for deadline := time.Now().Add(replayStack.Finish); time.Now().Before(deadline); time.Sleep(replayStack.Poll) {
		cur, err := svc.Status(ctx, routeID, rp.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !seen[cur.Status] {
			seen[cur.Status] = true
			t.Logf("%s %s group=%s partitions=%d error=%q", time.Now().Format(time.TimeOnly), cur.Status, cur.ConsumerGroup, cur.Partitions, cur.Error)
		}
		if cur.Status == domain.ReplayFailed {
			t.Fatalf("replay failed: %s", cur.Error)
		}
		if cur.Status == domain.ReplayDone {
			return
		}
	}
	t.Fatal("the live scheduler did not finish the replay in time")
}
