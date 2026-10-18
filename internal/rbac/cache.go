package rbac

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// SubjectCache keeps loaded subjects in-process; NOTIFY rbac_changed invalidates entries.
type SubjectCache interface {
	Get(id uuid.UUID) (Subject, bool)
	Put(s Subject)
	Invalidate(id uuid.UUID)
	InvalidateAll()
}

// Clock returns the current time (replaceable in tests).
type Clock func() time.Time

// TTLSubjectCache expires entries after a TTL (ROUTER_RBAC_CACHE_TTL, default 30 s).
type TTLSubjectCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	now     Clock
	entries map[uuid.UUID]cachedSubject
}

type cachedSubject struct {
	subject Subject
	expires time.Time
}

// NewTTLSubjectCache builds a cache; a nil clock uses time.Now.
func NewTTLSubjectCache(ttl time.Duration, now Clock) *TTLSubjectCache {
	if now == nil {
		now = time.Now
	}
	return &TTLSubjectCache{ttl: ttl, now: now, entries: map[uuid.UUID]cachedSubject{}}
}

// Get returns a non-expired subject.
func (c *TTLSubjectCache) Get(id uuid.UUID) (Subject, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[id]
	if !ok || !c.now().Before(e.expires) {
		return Subject{}, false
	}
	return e.subject, true
}

// Put stores s until now + TTL.
func (c *TTLSubjectCache) Put(s Subject) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[s.UserID] = cachedSubject{subject: s, expires: c.now().Add(c.ttl)}
}

// Invalidate drops one subject.
func (c *TTLSubjectCache) Invalidate(id uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, id)
}

// InvalidateAll drops every subject (after a listener reconnect, events may have been missed).
func (c *TTLSubjectCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[uuid.UUID]cachedSubject{}
}
