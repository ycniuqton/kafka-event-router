package store_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/testkit"
)

// pagedTeams is the row shape the keyset test pages over (sort key + id as text).
type pagedTeams struct {
	ID        string
	CreatedAt string
}

const (
	keysetRows     = 1000
	keysetPageSize = 37
	racers         = 10
)

// TestKeysetPaginationNoGapsNoDups pages 1k rows sharing timestamps through signed cursors.
func TestKeysetPaginationNoGapsNoDups(t *testing.T) {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	ctx := context.Background()
	_, err := sb.DB.Pool().Exec(ctx,
		`INSERT INTO teams (name) SELECT 'team-' || g FROM generate_series(1, $1) g`, keysetRows)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	pages := pageAll(t, sb.DB, func(row pagedTeams) {
		if seen[row.ID] {
			t.Fatalf("duplicate row %s", row.ID)
		}
		seen[row.ID] = true
	})
	if len(seen) != keysetRows {
		t.Fatalf("saw %d rows, want %d", len(seen), keysetRows)
	}
	if want := (keysetRows + keysetPageSize - 1) / keysetPageSize; pages != want {
		t.Fatalf("pages %d, want %d", pages, want)
	}
}

func pageAll(t *testing.T, db *store.DB, visit func(pagedTeams)) int {
	t.Helper()
	codec := store.NewCursorCodec("keyset-test")
	slicer := store.PageSlicer[pagedTeams]{Limit: keysetPageSize, KeyOf: func(r pagedTeams) store.Cursor {
		return store.Cursor{SortKey: []string{r.CreatedAt}, ID: r.ID}
	}}
	var after *store.Cursor
	for pages := 1; ; pages++ {
		rows := fetchPage(t, db, after)
		kept, next := slicer.Slice(rows)
		for _, r := range kept {
			visit(r)
		}
		if next == nil {
			return pages
		}
		decoded, err := codec.Decode(codec.Encode(*next))
		if err != nil {
			t.Fatal(err)
		}
		after = &decoded
	}
}

func fetchPage(t *testing.T, db *store.DB, after *store.Cursor) []pagedTeams {
	t.Helper()
	sql, args := store.KeysetQuery{
		Select:     `SELECT id::text, created_at::text FROM teams`,
		SortColumn: "created_at", SortCast: "timestamptz", IDColumn: "id", IDCast: "uuid",
		Limit: keysetPageSize, After: after,
	}.Build()
	rows, err := db.Pool().Query(context.Background(), sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []pagedTeams
	for rows.Next() {
		var r pagedTeams
		if err := rows.Scan(&r.ID, &r.CreatedAt); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

// TestVersionedUpdate covers bump, stale If-Match, missing row and a race on the same version.
func TestVersionedUpdate(t *testing.T) {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	ctx := context.Background()
	var id string
	if err := sb.DB.Pool().QueryRow(ctx, `INSERT INTO teams (name) VALUES ('v') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	upd := store.VersionedUpdate{Table: "teams", IDColumn: "id"}
	next, err := upd.Exec(ctx, sb.DB, "name = $1", []any{"v1"}, id, 1)
	if err != nil || next != 2 {
		t.Fatalf("update: %d %v", next, err)
	}
	_, err = upd.Exec(ctx, sb.DB, "name = $1", []any{"stale"}, id, 1)
	var pf *store.PreconditionFailedError
	if !errors.As(err, &pf) || pf.Current != 2 {
		t.Fatalf("stale: %v", err)
	}
	_, err = upd.Exec(ctx, sb.DB, "name = $1", []any{"x"}, "00000000-0000-0000-0000-000000000000", 1)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if wins := race(ctx, sb.DB, upd, id); wins != 1 {
		t.Fatalf("%d concurrent writers won on the same version, want 1", wins)
	}
}

func race(ctx context.Context, db *store.DB, upd store.VersionedUpdate, id string) int32 {
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := upd.Exec(ctx, db, "name = $1", []any{"race-" + string(rune('a'+i))}, id, 2); err == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	return wins.Load()
}

// TestTxManagerRollback checks that WithinTx joins nested calls and rolls back on error.
func TestTxManagerRollback(t *testing.T) {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	ctx := context.Background()
	txm := store.NewTxManager(sb.DB)
	boom := errors.New("boom")
	err := txm.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := sb.DB.Q(ctx).Exec(ctx, `INSERT INTO teams (name) VALUES ('rolled-back')`); err != nil {
			return err
		}
		return txm.WithinTx(ctx, func(context.Context) error { return boom })
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	var n int
	_ = sb.DB.Pool().QueryRow(ctx, `SELECT count(*) FROM teams WHERE name = 'rolled-back'`).Scan(&n)
	if n != 0 {
		t.Fatalf("rolled-back row visible (%d)", n)
	}
}
