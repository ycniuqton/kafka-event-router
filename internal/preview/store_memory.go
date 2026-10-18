package preview

import (
	"context"
	"sync"
	"time"

	"github.com/eventrouter/router/pkg/config"
)

// MemorySessionStore keeps sessions in process memory (tests, single-replica local runs).
type MemorySessionStore struct {
	mu       sync.Mutex
	now      func() time.Time
	sessions map[SessionID]*memorySession
}

type memorySession struct {
	session   Session
	records   []SampleRecord
	schema    *FieldSchema
	expiresAt time.Time
}

// NewMemorySessionStore builds an empty store using now for expiry.
func NewMemorySessionStore(now func() time.Time) *MemorySessionStore {
	return &MemorySessionStore{now: now, sessions: map[SessionID]*memorySession{}}
}

func (s *MemorySessionStore) Kind() StoreKind { return StoreMemory }

// Create stores the session and drops expired ones.
func (s *MemorySessionStore) Create(_ context.Context, sess Session, recs []SampleRecord, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep()
	s.sessions[sess.ID] = &memorySession{session: sess, records: append([]SampleRecord(nil), recs...), expiresAt: s.now().Add(ttl)}
	return nil
}

// Get returns the session or ErrSessionExpired.
func (s *MemorySessionStore) Get(_ context.Context, id SessionID) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.live(id)
	if err != nil {
		return Session{}, err
	}
	return m.session, nil
}

// Records returns a slice of the stored records.
func (s *MemorySessionStore) Records(_ context.Context, id SessionID, start, count int) ([]SampleRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.live(id)
	if err != nil {
		return nil, err
	}
	start = min(max(start, 0), len(m.records))
	end := min(start+max(count, 0), len(m.records))
	return append([]SampleRecord(nil), m.records[start:end]...), nil
}

// All returns every record.
func (s *MemorySessionStore) All(ctx context.Context, id SessionID) ([]SampleRecord, error) {
	return s.Records(ctx, id, 0, int(MaxEvents))
}

// PutSchema caches the schema.
func (s *MemorySessionStore) PutSchema(_ context.Context, id SessionID, fs FieldSchema) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.live(id)
	if err != nil {
		return err
	}
	m.schema = &fs
	return nil
}

// Schema returns the cached schema.
func (s *MemorySessionStore) Schema(_ context.Context, id SessionID) (FieldSchema, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.live(id)
	if err != nil || m.schema == nil {
		return FieldSchema{}, false, err
	}
	return *m.schema, true, nil
}

// Touch slides the expiry.
func (s *MemorySessionStore) Touch(_ context.Context, id SessionID, expiresAt time.Time, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.live(id)
	if err != nil {
		return err
	}
	m.session.ExpiresAt = expiresAt
	m.expiresAt = s.now().Add(ttl)
	return nil
}

// Reformat changes the format, drops the schema and slides the expiry.
func (s *MemorySessionStore) Reformat(_ context.Context, id SessionID, format config.FormatType, expiresAt time.Time, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.live(id)
	if err != nil {
		return err
	}
	m.session.Format, m.session.ExpiresAt, m.schema = format, expiresAt, nil
	m.expiresAt = s.now().Add(ttl)
	return nil
}

// Delete removes the session.
func (s *MemorySessionStore) Delete(_ context.Context, id SessionID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *MemorySessionStore) live(id SessionID) (*memorySession, error) {
	m, ok := s.sessions[id]
	if !ok || !s.now().Before(m.expiresAt) {
		delete(s.sessions, id)
		return nil, ErrSessionExpired
	}
	return m, nil
}

func (s *MemorySessionStore) sweep() {
	now := s.now()
	for id, m := range s.sessions {
		if !now.Before(m.expiresAt) {
			delete(s.sessions, id)
		}
	}
}
