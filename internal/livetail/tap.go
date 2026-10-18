package livetail

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"

	"github.com/eventrouter/router/internal/platform/logging"
)

// RecordSource builds the tail record of a finished record; it is only called for admitted records.
type RecordSource interface {
	TailRecord(limits TailLimits) TailRecord
}

// Tap is the worker side of the live tail: a cheap flag check, then a rate-limited copy to the sink.
type Tap interface {
	Active(routeID string) bool
	Offer(routeID string, src RecordSource)
	Run(ctx context.Context)
}

// NoopTap tails nothing (workers without Redis, tests).
type NoopTap struct{}

func (NoopTap) Active(string) bool         { return false }
func (NoopTap) Offer(string, RecordSource) {}
func (NoopTap) Run(context.Context)        {}

// DropReason tells why a tail record was not copied.
type DropReason string

const (
	DropRate  DropReason = "rate"
	DropQueue DropReason = "queue"
)

// DropCounter counts records the tap did not copy.
type DropCounter interface {
	Dropped(routeID string, reason DropReason)
}

// NoDrops ignores drops.
type NoDrops struct{}

func (NoDrops) Dropped(string, DropReason) {}

// TapLog names the tap's log messages.
type TapLog string

const (
	LogTapFlag  TapLog = "livetail: flag check failed"
	LogTapWrite TapLog = "livetail: stream write failed"
)

// streamEntry is one queued stream append.
type streamEntry struct {
	routeID string
	kind    EntryKind
	data    []byte
}

// RedisTap copies admitted records of watched routes into the route's capped Redis stream.
type RedisTap struct {
	client  redis.UniversalClient
	limits  TailLimits
	flags   *FlagCache
	buckets *RouteBuckets
	queue   chan streamEntry
	drops   DropCounter
	log     *slog.Logger
}

// NewRedisTap builds the tap; Run must be started to drain its queue.
func NewRedisTap(client redis.UniversalClient, limits TailLimits, drops DropCounter, log *slog.Logger) *RedisTap {
	if drops == nil {
		drops = NoDrops{}
	}
	return &RedisTap{client: client, limits: limits, flags: NewFlagCache(client, limits, log),
		buckets: NewRouteBuckets(limits, time.Now), queue: make(chan streamEntry, limits.QueueSize), drops: drops, log: log}
}

// Active implements Tap: a cached flag lookup refreshed in the background at most every FlagCacheTTL.
func (t *RedisTap) Active(routeID string) bool {
	return t.flags.Active(routeID)
}

// Offer implements Tap: over-rate records are dropped (one throttle notice per interval), others queued.
func (t *RedisTap) Offer(routeID string, src RecordSource) {
	if !t.buckets.Allow(routeID) {
		t.drops.Dropped(routeID, DropRate)
		if t.buckets.NoticeDue(routeID) {
			t.enqueue(streamEntry{routeID: routeID, kind: EntryThrottled, data: []byte(string(EntryThrottled))})
		}
		return
	}
	data, err := json.Marshal(src.TailRecord(t.limits))
	if err != nil {
		return
	}
	if !t.enqueue(streamEntry{routeID: routeID, kind: EntryRecord, data: data}) {
		t.drops.Dropped(routeID, DropQueue)
	}
}

// enqueue never blocks the runner: a full queue drops the entry.
func (t *RedisTap) enqueue(e streamEntry) bool {
	select {
	case t.queue <- e:
		return true
	default:
		return false
	}
}

// Run implements Tap: appends queued entries and keeps the flags of known routes fresh until ctx ends,
// so a quiet route is tailed from its first record after the flag appears.
func (t *RedisTap) Run(ctx context.Context) {
	sweep := time.NewTicker(t.limits.FlagCacheTTL)
	defer sweep.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-t.queue:
			t.write(ctx, e)
		case <-sweep.C:
			t.flags.RefreshKnown(ctx)
		}
	}
}

// write appends one entry (XADD MAXLEN ~) and refreshes the stream TTL.
func (t *RedisTap) write(ctx context.Context, e streamEntry) {
	ctx, cancel := context.WithTimeout(ctx, t.limits.RedisTimeout)
	defer cancel()
	key := TailKey{RouteID: e.routeID}.Stream()
	pipe := t.client.Pipeline()
	pipe.XAdd(ctx, &redis.XAddArgs{Stream: key, MaxLen: t.limits.StreamMaxLen, Approx: true,
		Values: []any{string(FieldKind), string(e.kind), string(FieldData), e.data}})
	pipe.Expire(ctx, key, t.limits.StreamTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		t.log.Warn(string(LogTapWrite), string(logging.AttrError), err)
	}
}

// flagState is the cached answer for one route.
type flagState struct {
	active    bool
	checkedAt time.Time
	checking  bool
}

// FlagCache answers "is this route watched?" from memory; stale entries refresh asynchronously, so the
// runner never waits on Redis (a route is tailed at most FlagCacheTTL after its flag appears).
type FlagCache struct {
	client redis.UniversalClient
	limits TailLimits
	log    *slog.Logger
	now    func() time.Time
	mu     sync.Mutex
	state  map[string]*flagState
}

// NewFlagCache builds an empty cache.
func NewFlagCache(client redis.UniversalClient, limits TailLimits, log *slog.Logger) *FlagCache {
	return &FlagCache{client: client, limits: limits, log: log, now: time.Now, state: map[string]*flagState{}}
}

// Active returns the cached flag and starts a refresh when it is stale.
func (c *FlagCache) Active(routeID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.state[routeID]
	if !ok {
		s = &flagState{}
		c.state[routeID] = s
	}
	if !s.checking && c.now().Sub(s.checkedAt) >= c.limits.FlagCacheTTL {
		s.checking = true
		go c.refresh(routeID)
	}
	return s.active
}

// RefreshKnown re-reads the flags of every route seen so far in one pipeline (no refresh in flight).
func (c *FlagCache) RefreshKnown(ctx context.Context) {
	routes := c.claimIdle()
	if len(routes) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, c.limits.RedisTimeout)
	defer cancel()
	pipe := c.client.Pipeline()
	checks := make([]*redis.IntCmd, len(routes))
	for i, routeID := range routes {
		checks[i] = pipe.Exists(ctx, TailKey{RouteID: routeID}.Flag())
	}
	if _, err := pipe.Exec(ctx); err != nil {
		c.log.Debug(string(LogTapFlag), string(logging.AttrError), err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, routeID := range routes {
		n, err := checks[i].Result()
		s := c.state[routeID]
		s.active, s.checkedAt, s.checking = err == nil && n > 0, c.now(), false
	}
}

// claimIdle marks every known route without a refresh in flight as checking and returns them.
func (c *FlagCache) claimIdle() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.state))
	for routeID, s := range c.state {
		if !s.checking {
			s.checking = true
			out = append(out, routeID)
		}
	}
	return out
}

// refresh reads the flag; a Redis error counts as "not watched".
func (c *FlagCache) refresh(routeID string) {
	ctx, cancel := context.WithTimeout(context.Background(), c.limits.RedisTimeout)
	defer cancel()
	n, err := c.client.Exists(ctx, TailKey{RouteID: routeID}.Flag()).Result()
	if err != nil {
		c.log.Debug(string(LogTapFlag), string(logging.AttrError), err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.state[routeID]
	s.active, s.checkedAt, s.checking = err == nil && n > 0, c.now(), false
}

// RouteBuckets holds one token bucket and throttle-notice clock per route.
type RouteBuckets struct {
	limits  TailLimits
	now     func() time.Time
	mu      sync.Mutex
	buckets map[string]*rate.Limiter
	notices map[string]time.Time
}

// NewRouteBuckets builds the per-route limiters (RatePerSec, Burst).
func NewRouteBuckets(limits TailLimits, now func() time.Time) *RouteBuckets {
	return &RouteBuckets{limits: limits, now: now, buckets: map[string]*rate.Limiter{}, notices: map[string]time.Time{}}
}

// Allow takes one token of the route's bucket.
func (b *RouteBuckets) Allow(routeID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.buckets[routeID]
	if !ok {
		l = rate.NewLimiter(rate.Limit(b.limits.RatePerSec), b.limits.Burst)
		b.buckets[routeID] = l
	}
	return l.AllowN(b.now(), 1)
}

// NoticeDue reports (and records) whether a throttle notice may be sent for the route now.
func (b *RouteBuckets) NoticeDue(routeID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if now.Sub(b.notices[routeID]) < b.limits.ThrottleEvery {
		return false
	}
	b.notices[routeID] = now
	return true
}

// TapKind names a tap implementation.
type TapKind string

const (
	TapRedis TapKind = "redis"
	TapNoop  TapKind = "noop"
)

// TapDeps are what tap factories may use.
type TapDeps struct {
	Redis  redis.UniversalClient
	Limits TailLimits
	Drops  DropCounter
	Log    *slog.Logger
}

// TapFactory builds one tap implementation.
type TapFactory func(d TapDeps) Tap

// TapManager selects the configured tap; Redis is only offered when a client exists.
type TapManager struct {
	factories map[TapKind]TapFactory
}

// NewTapManager registers the built-in taps.
func NewTapManager(hasRedis bool) *TapManager {
	m := &TapManager{factories: map[TapKind]TapFactory{TapNoop: func(TapDeps) Tap { return NoopTap{} }}}
	if hasRedis {
		m.factories[TapRedis] = func(d TapDeps) Tap { return NewRedisTap(d.Redis, d.Limits, d.Drops, d.Log) }
	}
	return m
}

// Register adds or replaces a tap implementation.
func (m *TapManager) Register(kind TapKind, f TapFactory) {
	m.factories[kind] = f
}

// Select builds the tap of kind.
func (m *TapManager) Select(kind TapKind, d TapDeps) (Tap, error) {
	f, ok := m.factories[kind]
	if !ok {
		return nil, fmt.Errorf("livetail: unknown or unavailable tap %q", kind)
	}
	return f(d), nil
}
