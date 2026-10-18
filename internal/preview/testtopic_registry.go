package preview

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/store"
)

// TestTopic is one temporary topic a test run wrote to; the janitor deletes it after ExpiresAt.
type TestTopic struct {
	ID           string
	RouteID      string
	ConnectionID string
	Topic        string
	Produced     int
	CreatedBy    string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

// TestTopicRegistry records test topics until the janitor deleted them from their cluster.
type TestTopicRegistry interface {
	Kind() TestTopicRegistryKind
	Record(ctx context.Context, t TestTopic) (TestTopic, error)
	Expired(ctx context.Context, now time.Time, limit int) ([]TestTopic, error)
	MarkDeleted(ctx context.Context, id string, at time.Time) error
}

// TestTopicRegistryFactory builds a registry implementation.
type TestTopicRegistryFactory func() (TestTopicRegistry, error)

// TestTopicRegistries selects the configured registry (ROUTER_PREVIEW_TEST_TOPICS).
type TestTopicRegistries struct {
	factories map[TestTopicRegistryKind]TestTopicRegistryFactory
}

// NewTestTopicRegistries returns an empty selection.
func NewTestTopicRegistries() *TestTopicRegistries {
	return &TestTopicRegistries{factories: map[TestTopicRegistryKind]TestTopicRegistryFactory{}}
}

// Register adds a factory for kind.
func (r *TestTopicRegistries) Register(kind TestTopicRegistryKind, f TestTopicRegistryFactory) {
	r.factories[kind] = f
}

// Build instantiates the registry of kind.
func (r *TestTopicRegistries) Build(kind TestTopicRegistryKind) (TestTopicRegistry, error) {
	f, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf("preview: no test topic registry registered for %q", kind)
	}
	return f()
}

// TestTopicSQL holds the statements of the Postgres registry (table preview_test_topics, migration 1070).
type TestTopicSQL string

const (
	sqlRecordTestTopic TestTopicSQL = `INSERT INTO preview_test_topics (route_id, connection_id, topic, produced, created_by, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (connection_id, topic) DO UPDATE SET produced = EXCLUDED.produced, expires_at = EXCLUDED.expires_at, deleted_at = NULL
RETURNING id`
	sqlExpiredTestTopics TestTopicSQL = `SELECT id, COALESCE(route_id::text, ''), connection_id, topic, produced, COALESCE(created_by::text, ''), created_at, expires_at
FROM preview_test_topics WHERE deleted_at IS NULL AND expires_at <= $1 ORDER BY expires_at LIMIT $2`
	sqlMarkTestTopicDeleted TestTopicSQL = `UPDATE preview_test_topics SET deleted_at = $2 WHERE id = $1`
)

// PostgresTestTopicRegistry keeps test topics in preview_test_topics so every replica's janitor sees them.
type PostgresTestTopicRegistry struct {
	db *store.DB
}

// NewPostgresTestTopicRegistry wraps the metadata database.
func NewPostgresTestTopicRegistry(db *store.DB) *PostgresTestTopicRegistry {
	return &PostgresTestTopicRegistry{db: db}
}

func (r *PostgresTestTopicRegistry) Kind() TestTopicRegistryKind { return TestTopicsPostgres }

// Record inserts (or re-arms) the topic row and returns it with its id.
func (r *PostgresTestTopicRegistry) Record(ctx context.Context, t TestTopic) (TestTopic, error) {
	ids := NullableUUIDs{}
	row := r.db.Q(ctx).QueryRow(ctx, string(sqlRecordTestTopic), ids.Of(t.RouteID), t.ConnectionID, t.Topic, t.Produced, ids.Of(t.CreatedBy), t.CreatedAt, t.ExpiresAt)
	var id uuid.UUID
	if err := row.Scan(&id); err != nil {
		return TestTopic{}, r.db.Translate(err)
	}
	t.ID = id.String()
	return t, nil
}

// Expired returns up to limit live topics whose expiry passed.
func (r *PostgresTestTopicRegistry) Expired(ctx context.Context, now time.Time, limit int) ([]TestTopic, error) {
	rows, err := r.db.Q(ctx).Query(ctx, string(sqlExpiredTestTopics), now, limit)
	if err != nil {
		return nil, r.db.Translate(err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (TestTopic, error) {
		var t TestTopic
		var id, conn uuid.UUID
		err := row.Scan(&id, &t.RouteID, &conn, &t.Topic, &t.Produced, &t.CreatedBy, &t.CreatedAt, &t.ExpiresAt)
		t.ID, t.ConnectionID = id.String(), conn.String()
		return t, err
	})
}

// MarkDeleted stamps the row so the janitor skips it.
func (r *PostgresTestTopicRegistry) MarkDeleted(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, string(sqlMarkTestTopicDeleted), id, at)
	return r.db.Translate(err)
}

// NullableUUIDs turns optional id strings into SQL values (empty or invalid → NULL).
type NullableUUIDs struct{}

// Of returns a uuid or nil.
func (NullableUUIDs) Of(s string) any {
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return id
}

// MemoryTestTopicRegistry keeps test topics in process memory (tests, single-replica local runs).
type MemoryTestTopicRegistry struct {
	mu     sync.Mutex
	topics map[memoryTestTopicKey]*memoryTestTopic
}

type memoryTestTopicKey struct{ connection, topic string }

type memoryTestTopic struct {
	topic   TestTopic
	deleted bool
}

// NewMemoryTestTopicRegistry returns an empty registry.
func NewMemoryTestTopicRegistry() *MemoryTestTopicRegistry {
	return &MemoryTestTopicRegistry{topics: map[memoryTestTopicKey]*memoryTestTopic{}}
}

func (r *MemoryTestTopicRegistry) Kind() TestTopicRegistryKind { return TestTopicsMemory }

// Record stores t keyed by connection and topic.
func (r *MemoryTestTopicRegistry) Record(_ context.Context, t TestTopic) (TestTopic, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := memoryTestTopicKey{connection: t.ConnectionID, topic: t.Topic}
	if prev, ok := r.topics[key]; ok {
		t.ID = prev.topic.ID
	} else {
		t.ID = uuid.NewString()
	}
	r.topics[key] = &memoryTestTopic{topic: t}
	return t, nil
}

// Expired returns live topics past their expiry, oldest first.
func (r *MemoryTestTopicRegistry) Expired(_ context.Context, now time.Time, limit int) ([]TestTopic, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []TestTopic{}
	for _, m := range r.topics {
		if !m.deleted && !m.topic.ExpiresAt.After(now) {
			out = append(out, m.topic)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
	return out[:min(limit, len(out))], nil
}

// MarkDeleted flags the topic as deleted.
func (r *MemoryTestTopicRegistry) MarkDeleted(_ context.Context, id string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.topics {
		if m.topic.ID == id {
			m.deleted = true
		}
	}
	return nil
}
