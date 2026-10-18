package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type txKey struct{}

// PgTxManager runs units of work in a pgx transaction carried in the context; nested calls join it.
type PgTxManager struct {
	db *DB
}

// NewTxManager returns a TxManager over the given DB.
func NewTxManager(db *DB) *PgTxManager {
	return &PgTxManager{db: db}
}

// WithinTx commits when fn succeeds and rolls back on error or panic.
func (m *PgTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	tx, err := m.db.pool.Begin(ctx)
	if err != nil {
		return m.db.Translate(err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()
	hooks := &commitHooks{}
	if err = fn(context.WithValue(context.WithValue(ctx, txKey{}, tx), commitHooksKey{}, hooks)); err != nil {
		return errors.Join(err, m.rollback(ctx, tx))
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	hooks.run()
	return nil
}

// InTx reports whether ctx carries an active transaction.
func (m *PgTxManager) InTx(ctx context.Context) bool {
	_, ok := ctx.Value(txKey{}).(pgx.Tx)
	return ok
}

func (m *PgTxManager) rollback(ctx context.Context, tx pgx.Tx) error {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fmt.Errorf("store: rollback: %w", err)
	}
	return nil
}
