package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// VersionedUpdate runs optimistic-concurrency updates on a table with a row_version column.
// The bump_row_version() trigger (0002) increments row_version on every UPDATE.
type VersionedUpdate struct {
	Table    string
	IDColumn string
}

// Exec applies `UPDATE … SET <set> WHERE id=$n AND row_version=$m RETURNING row_version`.
// setArgs are bound to $1..$k in set; a stale version yields *PreconditionFailedError.
func (v VersionedUpdate) Exec(ctx context.Context, db *DB, set string, setArgs []any, id any, expected RowVersion) (RowVersion, error) {
	n := len(setArgs)
	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d AND row_version = $%d RETURNING row_version",
		v.Table, set, v.IDColumn, n+1, n+2)
	args := append(append([]any{}, setArgs...), id, int64(expected))
	var next int64
	err := db.Q(ctx).QueryRow(ctx, sql, args...).Scan(&next)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, v.staleOrMissing(ctx, db, id)
	}
	if err != nil {
		return 0, db.Translate(err)
	}
	return RowVersion(next), nil
}

func (v VersionedUpdate) staleOrMissing(ctx context.Context, db *DB, id any) error {
	sql := fmt.Sprintf("SELECT row_version FROM %s WHERE %s = $1", v.Table, v.IDColumn)
	var current int64
	if err := db.Q(ctx).QueryRow(ctx, sql, id).Scan(&current); err != nil {
		return db.Translate(err)
	}
	return &PreconditionFailedError{Current: RowVersion(current)}
}
