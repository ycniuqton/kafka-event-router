package dlq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Lease holds acquired locks until Release.
type Lease interface {
	Release(ctx context.Context)
}

// RetryLock prevents two concurrent retries of the same event (NFR-6).
type RetryLock interface {
	Acquire(ctx context.Context, routeID string, refs []Ref) (Lease, []Ref, error)
}

// LockKeyPrefix is the Redis key prefix of retry locks.
type LockKeyPrefix string

const lockPrefix LockKeyPrefix = "dlq:lock:"

// lockTokenBytes is the random size of a lock owner token.
type lockTokenBytes int

const tokenSize lockTokenBytes = 16

// LockKey builds lock keys.
type LockKey struct{}

// Of returns the key of one event lock.
func (LockKey) Of(routeID string, r Ref) string {
	return string(lockPrefix) + (StateKey{}).Of(routeID, r)
}

// RedisRetryLock implements RetryLock with SET NX PX and owner-checked release.
type RedisRetryLock struct {
	Client redis.UniversalClient
}

// releaseScript deletes a key only when it still holds the owner's token.
var releaseScript = redis.NewScript(`if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`)

// Acquire implements RetryLock; refs already locked by someone else are returned as busy.
func (l RedisRetryLock) Acquire(ctx context.Context, routeID string, refs []Ref) (Lease, []Ref, error) {
	token := make([]byte, tokenSize)
	_, _ = rand.Read(token)
	lease := &redisLease{client: l.Client, token: hex.EncodeToString(token)}
	busy := []Ref{}
	for _, ref := range refs {
		key := (LockKey{}).Of(routeID, ref)
		ok, err := l.Client.SetNX(ctx, key, lease.token, LockTTL.Duration()).Result()
		if err != nil {
			lease.Release(ctx)
			return nil, nil, ErrUpstream.Wrap(err)
		}
		if !ok {
			busy = append(busy, ref)
			continue
		}
		lease.keys = append(lease.keys, key)
	}
	return lease, busy, nil
}

type redisLease struct {
	client redis.UniversalClient
	token  string
	keys   []string
}

// Release implements Lease.
func (l *redisLease) Release(ctx context.Context) {
	for _, k := range l.keys {
		releaseScript.Run(context.WithoutCancel(ctx), l.client, []string{k}, l.token)
	}
}

// MemoryRetryLock implements RetryLock in process memory (tests, single instance).
type MemoryRetryLock struct {
	mu   sync.Mutex
	held map[string]time.Time
	Now  func() time.Time
}

// NewMemoryRetryLock returns an empty lock table.
func NewMemoryRetryLock(now func() time.Time) *MemoryRetryLock {
	return &MemoryRetryLock{held: map[string]time.Time{}, Now: now}
}

// Acquire implements RetryLock.
func (l *MemoryRetryLock) Acquire(_ context.Context, routeID string, refs []Ref) (Lease, []Ref, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lease, busy := &memoryLease{owner: l}, []Ref{}
	for _, ref := range refs {
		key := (LockKey{}).Of(routeID, ref)
		if until, ok := l.held[key]; ok && l.Now().Before(until) {
			busy = append(busy, ref)
			continue
		}
		l.held[key] = l.Now().Add(LockTTL.Duration())
		lease.keys = append(lease.keys, key)
	}
	return lease, busy, nil
}

type memoryLease struct {
	owner *MemoryRetryLock
	keys  []string
}

// Release implements Lease.
func (l *memoryLease) Release(context.Context) {
	l.owner.mu.Lock()
	defer l.owner.mu.Unlock()
	for _, k := range l.keys {
		delete(l.owner.held, k)
	}
}
