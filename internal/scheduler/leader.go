// Package scheduler is the leader-only control loop inside cmd/api: lease reaping, replica placement,
// one-replica-at-a-time rollout with auto-rollback detection. Exactly one API instance is leader,
// elected with a Postgres session advisory lock.
package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// LockKey is the advisory lock id of the scheduler (7001 is the migration lock).
type LockKey int64

const SchedulerLockKey LockKey = 7008

// ErrNotLeader is returned when another instance holds the lock.
var ErrNotLeader = errors.New("scheduler: another instance is leader")

// LeaderElector campaigns for leadership; lost closes when leadership ends.
type LeaderElector interface {
	Campaign(ctx context.Context) (lost <-chan struct{}, err error)
}

// LeaderCheck is how often the leader verifies its lock connection.
type LeaderCheck time.Duration

const LeaderCheckInterval LeaderCheck = LeaderCheck(2 * time.Second)

// PgLeaderElector holds pg_try_advisory_lock on a dedicated pooled connection; losing the connection loses the lock.
type PgLeaderElector struct {
	Pool  *pgxpool.Pool
	Key   LockKey
	Gauge prometheus.Gauge
}

// Campaign tries once; on success it watches the session until ctx ends or the connection breaks.
func (e PgLeaderElector) Campaign(ctx context.Context) (<-chan struct{}, error) {
	conn, err := e.Pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	var won bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, int64(e.Key)).Scan(&won); err != nil || !won {
		conn.Release()
		return nil, errors.Join(err, ErrNotLeader)
	}
	e.Gauge.Set(1)
	lost := make(chan struct{})
	go func() {
		defer close(lost)
		defer e.Gauge.Set(0)
		defer conn.Release()
		e.hold(ctx, conn)
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, int64(e.Key))
	}()
	return lost, nil
}

// hold pings the lock session until it fails or ctx ends.
func (e PgLeaderElector) hold(ctx context.Context, conn *pgxpool.Conn) {
	t := time.NewTicker(time.Duration(LeaderCheckInterval))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if conn.Ping(ctx) != nil {
				return
			}
		}
	}
}

// AlwaysLeader makes the single instance leader (tests, single-API deployments).
type AlwaysLeader struct{}

// Campaign never loses until ctx ends.
func (AlwaysLeader) Campaign(ctx context.Context) (<-chan struct{}, error) {
	return ctx.Done(), nil
}
