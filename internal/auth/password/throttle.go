package password

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/ratelimit"
)

// Throttle is the base of login-attempt throttles: per-client rate plus per-account lockout.
type Throttle interface {
	Admit(ctx context.Context, r *http.Request, email string) error
	Failed(ctx context.Context, email string) error
	Succeeded(ctx context.Context, email string) error
}

// ThrottlePolicy configures the throttle: MaxFailures within Lockout locks the account for the rest of the window.
type ThrottlePolicy struct {
	MaxFailures int
	Lockout     time.Duration
	IPPerMinute int
}

// ThrottleKey prefixes the Redis keys of the throttle (emails are stored only as SHA-256).
type ThrottleKey string

const (
	KeyLoginFailures ThrottleKey = "login_fail:"
	KeyLoginClient   ThrottleKey = "login_ip:"
)

// BurstDivisor sets the per-client burst as a fraction of the per-minute rate.
type BurstDivisor int

const ClientBurstDivisor BurstDivisor = 2

// RedisThrottle implements Throttle on Redis, shared by every API replica; Redis outages fail closed.
type RedisThrottle struct {
	rdb     redis.UniversalClient
	limiter ratelimit.Limiter
	policy  ThrottlePolicy
}

// NewRedisThrottle builds the throttle; the client rate uses the platform's GCRA limiter.
func NewRedisThrottle(rdb redis.UniversalClient, limiter ratelimit.Limiter, policy ThrottlePolicy) *RedisThrottle {
	return &RedisThrottle{rdb: rdb, limiter: limiter, policy: policy}
}

// Admit rejects clients over their attempt rate and locked accounts with a LockedError.
func (t *RedisThrottle) Admit(ctx context.Context, r *http.Request, email string) error {
	rule := ratelimit.Rule{PerMinute: t.policy.IPPerMinute, Burst: max(1, t.policy.IPPerMinute/int(ClientBurstDivisor))}
	d, err := t.limiter.Allow(ctx, string(KeyLoginClient)+auth.ClientIP(r), rule)
	if err != nil {
		return auth.ErrAuthUnavailable.Wrap(err)
	}
	if !d.Allowed {
		return &LockedError{RetryAfter: d.RetryAfter}
	}
	return t.checkAccount(ctx, email)
}

func (t *RedisThrottle) checkAccount(ctx context.Context, email string) error {
	key := t.accountKey(email)
	n, err := t.rdb.Get(ctx, key).Int()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return auth.ErrAuthUnavailable.Wrap(err)
	}
	if n < t.policy.MaxFailures {
		return nil
	}
	ttl, err := t.rdb.PTTL(ctx, key).Result()
	if err != nil {
		return auth.ErrAuthUnavailable.Wrap(err)
	}
	return &LockedError{RetryAfter: max(ttl, time.Second)}
}

// Failed counts a failed attempt; the window starts with the first failure.
func (t *RedisThrottle) Failed(ctx context.Context, email string) error {
	key := t.accountKey(email)
	n, err := t.rdb.Incr(ctx, key).Result()
	if err != nil {
		return auth.ErrAuthUnavailable.Wrap(err)
	}
	if n == 1 {
		return t.rdb.PExpire(ctx, key, t.policy.Lockout).Err()
	}
	return nil
}

// Succeeded clears the account's failure count.
func (t *RedisThrottle) Succeeded(ctx context.Context, email string) error {
	return t.rdb.Del(ctx, t.accountKey(email)).Err()
}

func (t *RedisThrottle) accountKey(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return string(KeyLoginFailures) + hex.EncodeToString(sum[:])
}
