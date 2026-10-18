package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// KeyPrefix namespaces rate-limit keys in Redis.
type KeyPrefix string

const RedisKeyPrefix KeyPrefix = "rl:"

// gcraScript implements GCRA atomically on Redis server time: KEYS[1] holds the theoretical arrival time (ms);
// ARGV[1] = emission interval (ms), ARGV[2] = burst tolerance (ms). Returns {allowed, retry_after_ms}.
var gcraScript = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local interval = tonumber(ARGV[1])
local tolerance = tonumber(ARGV[2])
local tat = tonumber(redis.call('GET', KEYS[1]) or now)
if tat < now then tat = now end
local new_tat = tat + interval
local allow_at = new_tat - tolerance
if allow_at > now then
  return {0, allow_at - now}
end
redis.call('SET', KEYS[1], new_tat, 'PX', math.max(1, new_tat - now))
return {1, 0}
`)

// RedisLimiter is a GCRA token bucket shared by all replicas through Redis.
type RedisLimiter struct {
	rdb redis.UniversalClient
}

// NewRedisLimiter builds the limiter.
func NewRedisLimiter(rdb redis.UniversalClient) *RedisLimiter {
	return &RedisLimiter{rdb: rdb}
}

// Allow implements Limiter; burst requests may run back to back, then one per emission interval.
func (l *RedisLimiter) Allow(ctx context.Context, key string, rule Rule) (Decision, error) {
	interval := time.Minute.Milliseconds() / int64(max(rule.PerMinute, 1))
	tolerance := interval * int64(max(rule.Burst, 1))
	res, err := gcraScript.Run(ctx, l.rdb, []string{string(RedisKeyPrefix) + key}, interval, tolerance).Int64Slice()
	if err != nil {
		return Decision{}, err
	}
	return Decision{Allowed: res[0] == 1, RetryAfter: time.Duration(res[1]) * time.Millisecond}, nil
}
