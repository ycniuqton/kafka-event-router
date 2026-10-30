package enrich

import (
	"container/list"
	"sync"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
)

// CacheDefault holds the cache defaults of the lookup document.
type CacheDefault int

const DefaultCacheMaxEntries CacheDefault = 10000

// CacheSettings configures one answer cache; a zero TTL disables caching of that answer kind.
type CacheSettings struct {
	TTL         time.Duration
	NegativeTTL time.Duration
	MaxEntries  int
}

// CacheSettingsOf converts the document cache member (nil = no cache).
type CacheSettingsOf struct{}

// Of returns the settings and whether any caching is enabled.
func (CacheSettingsOf) Of(raw *config.LookupCache) (CacheSettings, bool) {
	if raw == nil {
		return CacheSettings{}, false
	}
	s := CacheSettings{MaxEntries: int(DefaultCacheMaxEntries)}
	if raw.TtlMs != nil {
		s.TTL = time.Duration(*raw.TtlMs) * time.Millisecond
	}
	if raw.NegativeTtlMs != nil {
		s.NegativeTTL = time.Duration(*raw.NegativeTtlMs) * time.Millisecond
	}
	if raw.MaxEntries != nil {
		s.MaxEntries = *raw.MaxEntries
	}
	return s, s.TTL > 0 || s.NegativeTTL > 0
}

// AnswerCache is a per-step LRU of source answers with separate TTLs for found answers and misses.
type AnswerCache struct {
	mu       sync.Mutex
	settings CacheSettings
	now      func() time.Time
	order    *list.List
	items    map[string]*list.Element
}

type cacheEntry struct {
	key     string
	answer  ctxhelpers.SourceAnswer
	expires time.Time
}

// NewAnswerCache builds a cache; now is the wall clock (injected in tests).
func NewAnswerCache(s CacheSettings, now func() time.Time) *AnswerCache {
	if now == nil {
		now = time.Now
	}
	return &AnswerCache{settings: s, now: now, order: list.New(), items: map[string]*list.Element{}}
}

// Get returns a live entry and refreshes its recency; expired entries are removed.
func (c *AnswerCache) Get(key string) (ctxhelpers.SourceAnswer, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return ctxhelpers.SourceAnswer{}, false
	}
	e := el.Value.(*cacheEntry)
	if !c.now().Before(e.expires) {
		c.remove(el)
		return ctxhelpers.SourceAnswer{}, false
	}
	c.order.MoveToFront(el)
	answer := e.answer
	answer.Value = engine.ValueCloner{}.Clone(answer.Value)
	return answer, true
}

// Put stores an answer with the TTL of its kind (found / miss), evicting the least recently used entry.
func (c *AnswerCache) Put(key string, answer ctxhelpers.SourceAnswer) {
	ttl := c.settings.NegativeTTL
	if answer.Found {
		ttl = c.settings.TTL
	}
	if ttl <= 0 || c.settings.MaxEntries <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.remove(el)
	}
	entry := &cacheEntry{key: key, answer: answer, expires: c.now().Add(ttl)}
	c.items[key] = c.order.PushFront(entry)
	for c.order.Len() > c.settings.MaxEntries {
		c.remove(c.order.Back())
	}
}

// Len returns the number of stored entries (live or not yet evicted).
func (c *AnswerCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

func (c *AnswerCache) remove(el *list.Element) {
	c.order.Remove(el)
	delete(c.items, el.Value.(*cacheEntry).key)
}
