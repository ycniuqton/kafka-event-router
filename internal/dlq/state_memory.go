package dlq

import (
	"context"
	"sync"
)

// MemoryStateStore keeps resolved state in process memory (tests, ROUTER_DLQ_STATE_BACKEND=memory).
type MemoryStateStore struct {
	mu      sync.Mutex
	indexes map[string]*StateIndex
}

// NewMemoryStateStore returns an empty store.
func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{indexes: map[string]*StateIndex{}}
}

func (s *MemoryStateStore) index(conn string) *StateIndex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.indexes[conn] == nil {
		s.indexes[conn] = NewStateIndex()
	}
	return s.indexes[conn]
}

// Lookup implements StateStore.
func (s *MemoryStateStore) Lookup(_ context.Context, conn, routeID string, refs []Ref) (map[Ref]Resolution, error) {
	return s.index(conn).Lookup(routeID, refs), nil
}

// Mark implements StateStore.
func (s *MemoryStateStore) Mark(_ context.Context, conn string, rs []Resolution) error {
	for _, r := range rs {
		s.index(conn).Put(r)
	}
	return nil
}

// Clear implements StateStore.
func (s *MemoryStateStore) Clear(_ context.Context, conn, routeID string, refs []Ref) error {
	for _, ref := range refs {
		s.index(conn).Delete(routeID, ref)
	}
	return nil
}

// CountResolved implements StateStore.
func (s *MemoryStateStore) CountResolved(_ context.Context, conn, routeID string, ranges []PartitionRange) (int, error) {
	return s.index(conn).Count(routeID, ranges), nil
}

// Expired implements StateStore.
func (s *MemoryStateStore) Expired(_ context.Context, conn, routeID string, ranges []PartitionRange) ([]Ref, error) {
	return s.index(conn).Below(routeID, ranges), nil
}
