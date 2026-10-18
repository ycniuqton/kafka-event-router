package store

import (
	"context"
	"fmt"

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

// DB owns the pgx pool and resolves the ambient transaction from the context.
type DB struct {
	pool        *pgxpool.Pool
	constraints *ConstraintMap
}

// Open connects a pgx pool to the configured database (lazy: the first query dials).
func Open(ctx context.Context, cfg config.DatabaseConfig) (*DB, error) {
	pool, err := pgxpool.New(ctx, cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("store: open pool: %w", err)
	}
	return &DB{pool: pool, constraints: NewConstraintMap()}, nil
}

// Q returns the transaction carried by ctx, or the pool when no transaction is active.
func (d *DB) Q(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return d.pool
}

// Pool exposes the raw pool for LISTEN connections and migrations.
func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
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

// Check implements httpserver.ReadinessCheck by pinging the pool.
func (d *DB) Check(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

// Close releases every pooled connection.
func (d *DB) Close() {
	d.pool.Close()
}
