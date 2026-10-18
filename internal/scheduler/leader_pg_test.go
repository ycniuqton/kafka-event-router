// Integration test of PgLeaderElector on a real Postgres (RR-57): only one leader, and failover within
// 10 s after the leader's database session dies. Opt-in: set ROUTER_IT_DATABASE_URL (e.g. the dev stack DSN).
package scheduler_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventrouter/router/internal/scheduler"
)

// leaderIT holds the knobs of the leader integration test; the lock key differs from the live scheduler's.
var leaderIT = struct {
	EnvDSN   string
	Key      scheduler.LockKey
	Failover time.Duration
	Retry    time.Duration
}{EnvDSN: "ROUTER_IT_DATABASE_URL", Key: 7_008_957, Failover: 10 * time.Second, Retry: 250 * time.Millisecond}

// pgInstance is one simulated API replica: its own pool and elector.
type pgInstance struct {
	pool    *pgxpool.Pool
	elector scheduler.PgLeaderElector
}

func newPgInstance(t *testing.T, dsn string) pgInstance {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "router_scheduler_leader"})
	return pgInstance{pool: pool, elector: scheduler.PgLeaderElector{Pool: pool, Key: leaderIT.Key, Gauge: gauge}}
}

// lockHolder returns the backend pid holding the test advisory lock.
func lockHolder(t *testing.T, pool *pgxpool.Pool) int32 {
	t.Helper()
	var pid int32
	err := pool.QueryRow(context.Background(),
		`SELECT pid FROM pg_locks WHERE locktype = 'advisory' AND granted AND classid = 0 AND objid = $1 AND objsubid = 1`,
		int64(leaderIT.Key)).Scan(&pid)
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func TestPgLeaderElectionSingleLeaderAndFailover(t *testing.T) {
	dsn := os.Getenv(leaderIT.EnvDSN)
	if dsn == "" {
		t.Skipf("set %s to run against a real Postgres", leaderIT.EnvDSN)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := newPgInstance(t, dsn), newPgInstance(t, dsn)
	lostA, err := a.elector.Campaign(ctx)
	if err != nil {
		t.Fatalf("first instance did not become leader: %v", err)
	}
	if _, err := b.elector.Campaign(ctx); !errors.Is(err, scheduler.ErrNotLeader) {
		t.Fatalf("second instance campaign = %v, want ErrNotLeader", err)
	}
	start := time.Now()
	if _, err := b.pool.Exec(ctx, `SELECT pg_terminate_backend($1)`, lockHolder(t, b.pool)); err != nil {
		t.Fatal(err)
	}
	lostB := campaignUntil(t, ctx, b.elector, start.Add(leaderIT.Failover))
	t.Logf("failover after %s", time.Since(start).Round(time.Millisecond))
	select {
	case <-lostA:
	case <-time.After(leaderIT.Failover):
		t.Fatal("old leader never noticed it lost its session")
	}
	if _, err := a.elector.Campaign(ctx); !errors.Is(err, scheduler.ErrNotLeader) {
		t.Fatalf("old leader re-campaign = %v, want ErrNotLeader while the new leader holds the lock", err)
	}
	cancel()
	<-lostB
}

// campaignUntil retries like a standby scheduler until it wins or the deadline passes.
func campaignUntil(t *testing.T, ctx context.Context, e scheduler.PgLeaderElector, deadline time.Time) <-chan struct{} {
	t.Helper()
	for time.Now().Before(deadline) {
		lost, err := e.Campaign(ctx)
		if err == nil {
			return lost
		}
		time.Sleep(leaderIT.Retry)
	}
	t.Fatalf("no failover within %s", leaderIT.Failover)
	return nil
}
