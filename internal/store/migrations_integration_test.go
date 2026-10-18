package store_test

import (
	"context"
	"testing"

	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/testkit"
)

// TestMigrationsUpDownUp applies every migration, reverts them one by one, re-applies them and then
// checks the schema invariants (row_version trigger, append-only audit_log).
func TestMigrationsUpDownUp(t *testing.T) {
	sb := testkit.NewManager().Postgres(t)
	ctx := context.Background()
	mg := sb.Migrator()
	sb.Migrate(t)
	assertApplied(t, mg, mg.Latest())
	for st := statusOf(t, mg); st.Applied > 0; st = statusOf(t, mg) {
		if err := mg.Down(ctx, 1); err != nil {
			t.Fatalf("down from %d: %v", st.Applied, err)
		}
	}
	if n := userTables(t, sb); n != 0 {
		t.Fatalf("%d tables left after all downs", n)
	}
	sb.Migrate(t)
	assertApplied(t, mg, mg.Latest())
	assertRowVersionTrigger(t, sb)
	assertAuditAppendOnly(t, sb)
}

func statusOf(t *testing.T, mg *store.Migrator) store.MigrationStatus {
	t.Helper()
	st, err := mg.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Dirty {
		t.Fatalf("schema dirty at %d", st.Applied)
	}
	return st
}

func assertApplied(t *testing.T, mg *store.Migrator, want uint) {
	t.Helper()
	if st := statusOf(t, mg); st.Applied != want {
		t.Fatalf("applied %d, want %d", st.Applied, want)
	}
}

func userTables(t *testing.T, sb *testkit.PostgresSandbox) int {
	t.Helper()
	var n int
	err := sb.DB.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name <> 'schema_migrations'`,
		sb.Schema).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func assertRowVersionTrigger(t *testing.T, sb *testkit.PostgresSandbox) {
	t.Helper()
	ctx := context.Background()
	var id string
	var v1, v2 int64
	q := sb.DB.Pool()
	if err := q.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('rv') RETURNING id::text, row_version`).Scan(&id, &v1); err != nil {
		t.Fatal(err)
	}
	if err := q.QueryRow(ctx, `UPDATE teams SET name = 'rv2' WHERE id = $1 RETURNING row_version`, id).Scan(&v2); err != nil {
		t.Fatal(err)
	}
	if v1 != 1 || v2 != 2 {
		t.Fatalf("row_version %d -> %d, want 1 -> 2", v1, v2)
	}
}

func assertAuditAppendOnly(t *testing.T, sb *testkit.PostgresSandbox) {
	t.Helper()
	ctx := context.Background()
	q := sb.DB.Pool()
	if _, err := q.Exec(ctx, `INSERT INTO audit_log (action, object_type) VALUES ('test.append', 'team')`); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Exec(ctx, `UPDATE audit_log SET action = 'x'`); err == nil {
		t.Fatal("audit_log UPDATE must be rejected")
	}
	if _, err := q.Exec(ctx, `DELETE FROM audit_log`); err == nil {
		t.Fatal("audit_log DELETE must be rejected")
	}
}
