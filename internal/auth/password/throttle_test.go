package password

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/platform/ratelimit"
)

func newThrottle(t *testing.T, policy ThrottlePolicy) (*RedisThrottle, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewRedisThrottle(rdb, ratelimit.NewRedisLimiter(rdb), policy), mr
}

func TestThrottleLocksAccountAfterMaxFailures(t *testing.T) {
	th, mr := newThrottle(t, ThrottlePolicy{MaxFailures: 3, Lockout: 10 * time.Minute, IPPerMinute: 1000})
	ctx, r := context.Background(), httptest.NewRequest("POST", "/", nil)
	for i := 0; i < 3; i++ {
		if err := th.Admit(ctx, r, "ada@example.com"); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if err := th.Failed(ctx, "ADA@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	var locked *LockedError
	if err := th.Admit(ctx, r, "ada@example.com"); !errors.As(err, &locked) || locked.RetryAfter > 10*time.Minute {
		t.Fatalf("want locked, got %v", err)
	}
	if err := th.Admit(ctx, r, "bob@example.com"); err != nil {
		t.Fatalf("other accounts stay open: %v", err)
	}
	mr.FastForward(11 * time.Minute)
	if err := th.Admit(ctx, r, "ada@example.com"); err != nil {
		t.Fatalf("lock must expire: %v", err)
	}
}

func TestThrottleSuccessClearsFailures(t *testing.T) {
	th, _ := newThrottle(t, ThrottlePolicy{MaxFailures: 2, Lockout: time.Minute, IPPerMinute: 1000})
	ctx, r := context.Background(), httptest.NewRequest("POST", "/", nil)
	_ = th.Failed(ctx, "ada@example.com")
	_ = th.Succeeded(ctx, "ada@example.com")
	_ = th.Failed(ctx, "ada@example.com")
	if err := th.Admit(ctx, r, "ada@example.com"); err != nil {
		t.Fatalf("one failure after success must not lock: %v", err)
	}
}

func TestThrottleLimitsClientRate(t *testing.T) {
	th, _ := newThrottle(t, ThrottlePolicy{MaxFailures: 100, Lockout: time.Minute, IPPerMinute: 4})
	ctx, r := context.Background(), httptest.NewRequest("POST", "/", nil)
	var locked *LockedError
	allowed := 0
	for i := 0; i < 6; i++ {
		err := th.Admit(ctx, r, "user"+string(rune('a'+i))+"@example.com")
		if err == nil {
			allowed++
		} else if !errors.As(err, &locked) {
			t.Fatal(err)
		}
	}
	if allowed != 2 || locked == nil {
		t.Fatalf("allowed=%d locked=%v", allowed, locked)
	}
}

func TestThrottleFailsClosedWhenRedisDown(t *testing.T) {
	th, mr := newThrottle(t, ThrottlePolicy{MaxFailures: 3, Lockout: time.Minute, IPPerMinute: 10})
	mr.Close()
	if err := th.Admit(context.Background(), httptest.NewRequest("POST", "/", nil), "ada@example.com"); err == nil {
		t.Fatal("redis outage must not allow login attempts")
	}
}
