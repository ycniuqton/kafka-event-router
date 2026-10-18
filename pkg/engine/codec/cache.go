package codec

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// DefaultSchemaCacheSize bounds cached schemas per registry and parsed schemas per codec.
const DefaultSchemaCacheSize = 1024

// LatestTTL is how long a subject's latest version is reused for encoding.
const LatestTTL = time.Minute

// LRU is a size-bounded, concurrency-safe least-recently-used cache.
type LRU[K comparable, V any] struct {
	mu    sync.Mutex
	size  int
	order *list.List
	items map[K]*list.Element
}

type lruEntry[K comparable, V any] struct {
	key   K
	value V
}

// NewLRU returns a cache holding at most size entries.
func NewLRU[K comparable, V any](size int) *LRU[K, V] {
	return &LRU[K, V]{size: max(size, 1), order: list.New(), items: map[K]*list.Element{}}
}

// Get returns the cached value and marks it recently used.
func (c *LRU[K, V]) Get(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		c.order.MoveToFront(e)
		return e.Value.(lruEntry[K, V]).value, true
	}
	var zero V
	return zero, false
}

// Put stores a value, evicting the least recently used entry when full.
func (c *LRU[K, V]) Put(k K, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		e.Value = lruEntry[K, V]{key: k, value: v}
		c.order.MoveToFront(e)
		return
	}
	c.items[k] = c.order.PushFront(lruEntry[K, V]{key: k, value: v})
	if c.order.Len() > c.size {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.items, last.Value.(lruEntry[K, V]).key)
	}
}

// Len returns the number of cached entries.
func (c *LRU[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// subjectVersion keys immutable subject versions.
type subjectVersion struct {
	subject string
	version int
}

// latestEntry is a subject's latest schema with its fetch time.
type latestEntry struct {
	schema  RegisteredSchema
	fetched time.Time
}

// CachedRegistry caches immutable lookups (by id, by subject version) without TTL and
// the latest version of a subject for LatestTTL.
type CachedRegistry struct {
	inner    SchemaRegistry
	byID     *LRU[int32, RegisteredSchema]
	versions *LRU[subjectVersion, RegisteredSchema]
	latest   *LRU[string, latestEntry]
	now      func() time.Time
}

// NewCachedRegistry wraps inner with caches of the given size.
func NewCachedRegistry(inner SchemaRegistry, size int) *CachedRegistry {
	return &CachedRegistry{inner: inner, byID: NewLRU[int32, RegisteredSchema](size),
		versions: NewLRU[subjectVersion, RegisteredSchema](size), latest: NewLRU[string, latestEntry](size), now: time.Now}
}

func (c *CachedRegistry) SchemaByID(ctx context.Context, id int32) (RegisteredSchema, error) {
	if s, ok := c.byID.Get(id); ok {
		return s, nil
	}
	s, err := c.inner.SchemaByID(ctx, id)
	if err == nil {
		c.byID.Put(id, s)
	}
	return s, err
}

func (c *CachedRegistry) Version(ctx context.Context, subject string, version int) (RegisteredSchema, error) {
	k := subjectVersion{subject: subject, version: version}
	if s, ok := c.versions.Get(k); ok {
		return s, nil
	}
	s, err := c.inner.Version(ctx, subject, version)
	if err == nil {
		c.versions.Put(k, s)
	}
	return s, err
}

func (c *CachedRegistry) Latest(ctx context.Context, subject string) (RegisteredSchema, error) {
	if e, ok := c.latest.Get(subject); ok && c.now().Sub(e.fetched) < LatestTTL {
		return e.schema, nil
	}
	s, err := c.inner.Latest(ctx, subject)
	if err == nil {
		c.latest.Put(subject, latestEntry{schema: s, fetched: c.now()})
	}
	return s, err
}
