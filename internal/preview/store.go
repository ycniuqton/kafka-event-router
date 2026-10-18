package preview

import (
	"context"
	"fmt"
	"time"

	"github.com/eventrouter/router/pkg/config"
)

// SessionStore keeps sessions, their records and inferred schema with a sliding TTL.
// Get on an unknown or expired id returns ErrSessionExpired.
type SessionStore interface {
	Kind() StoreKind
	Create(ctx context.Context, s Session, recs []SampleRecord, ttl time.Duration) error
	Get(ctx context.Context, id SessionID) (Session, error)
	Records(ctx context.Context, id SessionID, start, count int) ([]SampleRecord, error)
	All(ctx context.Context, id SessionID) ([]SampleRecord, error)
	PutSchema(ctx context.Context, id SessionID, fs FieldSchema) error
	Schema(ctx context.Context, id SessionID) (FieldSchema, bool, error)
	Touch(ctx context.Context, id SessionID, expiresAt time.Time, ttl time.Duration) error
	Reformat(ctx context.Context, id SessionID, format config.FormatType, expiresAt time.Time, ttl time.Duration) error
	Delete(ctx context.Context, id SessionID) error
}

// SessionStoreFactory builds a store implementation.
type SessionStoreFactory func() (SessionStore, error)

// SessionStoreRegistry selects the configured store (ROUTER_PREVIEW_SESSION_STORE).
type SessionStoreRegistry struct {
	factories map[StoreKind]SessionStoreFactory
}

// NewSessionStoreRegistry returns an empty registry.
func NewSessionStoreRegistry() *SessionStoreRegistry {
	return &SessionStoreRegistry{factories: map[StoreKind]SessionStoreFactory{}}
}

// Register adds a factory for kind.
func (r *SessionStoreRegistry) Register(kind StoreKind, f SessionStoreFactory) {
	r.factories[kind] = f
}

// Build instantiates the store of kind.
func (r *SessionStoreRegistry) Build(kind StoreKind) (SessionStore, error) {
	f, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf("preview: no session store registered for %q", kind)
	}
	return f()
}

// SessionStoreManager is the only entry to sessions: TTL policy, sliding expiry and owner checks.
type SessionStoreManager struct {
	active SessionStore
	ttl    time.Duration
	now    func() time.Time
}

// NewSessionStoreManager wraps the active store with the session TTL policy.
func NewSessionStoreManager(active SessionStore, now func() time.Time) *SessionStoreManager {
	return &SessionStoreManager{active: active, ttl: SessionTTL.Duration(), now: now}
}

// Create stamps creation/expiry times and stores the session.
func (m *SessionStoreManager) Create(ctx context.Context, s Session, recs []SampleRecord) (Session, error) {
	s.CreatedAt = m.now().UTC()
	s.ExpiresAt = s.CreatedAt.Add(m.ttl)
	return s, m.active.Create(ctx, s, recs, m.ttl)
}

// Owned returns the session when caller owns it; foreign sessions look like missing ones.
func (m *SessionStoreManager) Owned(ctx context.Context, id SessionID, caller Caller) (Session, error) {
	s, err := m.active.Get(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if s.OwnerID != caller.UserID {
		return Session{}, ErrSessionNotFound
	}
	return s, nil
}

// Touch slides the session TTL to a full period from now.
func (m *SessionStoreManager) Touch(ctx context.Context, s Session) (Session, error) {
	s.ExpiresAt = m.now().UTC().Add(m.ttl)
	return s, m.active.Touch(ctx, s.ID, s.ExpiresAt, m.ttl)
}

// Reformat switches the session's value format (raw bytes are re-decoded on read), drops the cached
// schema and slides the TTL.
func (m *SessionStoreManager) Reformat(ctx context.Context, s Session, format config.FormatType) (Session, error) {
	s.Format, s.ExpiresAt = format, m.now().UTC().Add(m.ttl)
	return s, m.active.Reformat(ctx, s.ID, format, s.ExpiresAt, m.ttl)
}

// Records returns count records starting at start.
func (m *SessionStoreManager) Records(ctx context.Context, id SessionID, start, count int) ([]SampleRecord, error) {
	return m.active.Records(ctx, id, start, count)
}

// All returns every record of the session.
func (m *SessionStoreManager) All(ctx context.Context, id SessionID) ([]SampleRecord, error) {
	return m.active.All(ctx, id)
}

// PutSchema caches the inferred schema.
func (m *SessionStoreManager) PutSchema(ctx context.Context, id SessionID, fs FieldSchema) error {
	return m.active.PutSchema(ctx, id, fs)
}

// Schema returns the cached schema, if inferred already.
func (m *SessionStoreManager) Schema(ctx context.Context, id SessionID) (FieldSchema, bool, error) {
	return m.active.Schema(ctx, id)
}

// Delete removes the session.
func (m *SessionStoreManager) Delete(ctx context.Context, id SessionID) error {
	return m.active.Delete(ctx, id)
}
