package testkit

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/store"
)

// SandboxNaming builds the per-test schema name and the connection parameter that pins it.
type SandboxNaming string

const (
	SandboxSchemaPrefix SandboxNaming = "testkit_"
	SearchPathParam     SandboxNaming = "search_path"
)

// SandboxTiming bounds how long the sandbox waits for Postgres to accept queries.
type SandboxTiming time.Duration

const (
	SandboxReadyTimeout SandboxTiming = SandboxTiming(60 * time.Second)
	SandboxPollInterval SandboxTiming = SandboxTiming(500 * time.Millisecond)
)

// PostgresSandbox is one isolated schema whose search_path holds only that schema, so migrations
// and queries never see (or drop) objects of the shared database; it is dropped at test cleanup.
type PostgresSandbox struct {
	DB     *store.DB
	URL    string
	Schema string
}

// Postgres creates an empty sandbox schema on the provided server and opens a store.DB on it.
func (m *Manager) Postgres(t testing.TB) *PostgresSandbox {
	t.Helper()
	ep := m.Require(t, ServicePostgres)
	ctx := context.Background()
	admin := openReady(t, ep.URL)
	t.Cleanup(admin.Close)
	schema := string(SandboxSchemaPrefix) + strings.ToLower(ulid.Make().String())
	if _, err := admin.Pool().Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("testkit: create schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Pool().Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE") })
	sandboxURL := withSearchPath(t, ep.URL, schema)
	db := openReady(t, sandboxURL)
	t.Cleanup(db.Close)
	return &PostgresSandbox{DB: db, URL: sandboxURL, Schema: schema}
}

// Migrator returns the production migrator bound to the sandbox schema.
func (s *PostgresSandbox) Migrator() *store.Migrator {
	return store.NewMigrator(s.DB, s.URL)
}

// Migrate applies every embedded migration to the sandbox.
func (s *PostgresSandbox) Migrate(t testing.TB) {
	t.Helper()
	if err := s.Migrator().Up(context.Background()); err != nil {
		t.Fatalf("testkit: migrate up: %v", err)
	}
}

func openReady(t testing.TB, dsn string) *store.DB {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{URL: dsn})
	if err != nil {
		t.Fatalf("testkit: open: %v", err)
	}
	deadline := time.Now().Add(time.Duration(SandboxReadyTimeout))
	for err = db.Check(ctx); err != nil && time.Now().Before(deadline); err = db.Check(ctx) {
		time.Sleep(time.Duration(SandboxPollInterval))
	}
	if err != nil {
		db.Close()
		t.Fatalf("testkit: postgres not ready: %v", err)
	}
	return db
}

func withSearchPath(t testing.TB, dsn, schema string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("testkit: parse database url: %v", err)
	}
	q := u.Query()
	q.Set(string(SearchPathParam), schema)
	u.RawQuery = q.Encode()
	return u.String()
}
