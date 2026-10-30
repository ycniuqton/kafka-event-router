package store

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventrouter/router/internal/platform/config"
)

// Querier is the statement surface shared by the pool and a transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CheckName is the readiness check name of the metadata database.
type CheckName string

const CheckPostgres CheckName = "postgres"

// DB owns the pgx pool for statements plus the dedicated sessions that must never compete with it:
// the process listener hub and the readiness probe. It resolves the ambient transaction from the context.
type DB struct {
	pool        *pgxpool.Pool
	constraints *ConstraintMap
	hub         *ListenerHub
	health      *HealthProbe
}

// Open connects a pgx pool to the configured database (lazy: the first query dials).
func Open(ctx context.Context, cfg config.DatabaseConfig) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("store: open pool: %w", err)
	}
	PoolSizing{}.Apply(poolCfg, cfg)
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("store: open pool: %w", err)
	}
	sessions := SessionDialerOf(pool)
	return &DB{pool: pool, constraints: NewConstraintMap(), hub: NewListenerHub(sessions, DefaultListenerTimings(), nil),
		health: NewHealthProbe(sessions)}, nil
}

// PoolLimit names the pool sizing defaults.
type PoolLimit int

const (
	MinPoolMaxConns PoolLimit = 10
	PoolConnsPerCPU PoolLimit = 2
)

// PoolParam names DSN parameters pgxpool reads itself.
type PoolParam string

const PoolMaxConnsParam PoolParam = "pool_max_conns"

// PoolSizing decides the pool's max connections: DSN parameter, then ROUTER_DB_POOL_MAX_CONNS, then the default.
type PoolSizing struct{}

// Apply sets MaxConns on poolCfg unless the DSN already carries pool_max_conns.
func (PoolSizing) Apply(poolCfg *pgxpool.Config, cfg config.DatabaseConfig) {
	if strings.Contains(cfg.URL, string(PoolMaxConnsParam)) {
		return
	}
	poolCfg.MaxConns = int32(PoolSizing{}.Default(cfg.PoolMaxConns, runtime.NumCPU()))
}

// Default returns configured when positive, else max(MinPoolMaxConns, PoolConnsPerCPU × cpus).
func (PoolSizing) Default(configured, cpus int) int {
	if configured > 0 {
		return configured
	}
	return max(int(MinPoolMaxConns), int(PoolConnsPerCPU)*cpus)
}

// Q returns the transaction carried by ctx, or the pool when no transaction is active.
func (d *DB) Q(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return d.pool
}

// Pool exposes the raw pool for migrations and tests; long-lived sessions use Notifications or SessionDialerOf.
func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}

// Notifications returns the process listener hub (one dedicated LISTEN session shared by all subscribers).
func (d *DB) Notifications() *ListenerHub {
	return d.hub
}

// Constraints returns the constraint → JSON Pointer map used to translate unique violations.
func (d *DB) Constraints() *ConstraintMap {
	return d.constraints
}

// Translate converts driver errors into store errors (not found, conflict with field path).
func (d *DB) Translate(err error) error {
	return d.constraints.Translate(err)
}

// Name implements httpserver.ReadinessCheck.
func (d *DB) Name() string {
	return string(CheckPostgres)
}

// Check implements httpserver.ReadinessCheck by pinging on the dedicated health session (a busy pool never fails it).
func (d *DB) Check(ctx context.Context) error {
	return d.health.Check(ctx)
}

// Close stops the listener hub and the health session and releases every pooled connection.
func (d *DB) Close() {
	d.hub.Close()
	d.health.Close()
	d.pool.Close()
}
