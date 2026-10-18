package dlq

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisRetryLockAndSummaryCache(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	lock := RedisRetryLock{Client: client}
	refs := []Ref{{Topic: "t", Offset: 1}, {Topic: "t", Offset: 2}}
	lease, busy, err := lock.Acquire(ctx, "r", refs[:1])
	if err != nil || len(busy) != 0 {
		t.Fatal(err, busy)
	}
	other, busy, _ := lock.Acquire(ctx, "r", refs)
	if len(busy) != 1 || busy[0] != refs[0] {
		t.Fatalf("busy %+v", busy)
	}
	lease.Release(ctx)
	other.Release(ctx)
	if _, busy, _ := lock.Acquire(ctx, "r", refs); len(busy) != 0 {
		t.Fatal("not released")
	}
	cache := RedisSummaryCache{Client: client}
	cache.Set(ctx, "r", Summary{Unresolved: 3})
	if s, ok := cache.Get(ctx, "r"); !ok || s.Unresolved != 3 {
		t.Fatal("cache get")
	}
	cache.Invalidate(ctx, "r")
	if _, ok := cache.Get(ctx, "r"); ok {
		t.Fatal("cache invalidate")
	}
}
