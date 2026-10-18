package connections

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RatePolicy holds the rate-sampling constants.
type RatePolicy time.Duration

const (
	RateMinInterval RatePolicy = RatePolicy(5 * time.Second)
	RateSnapshotTTL RatePolicy = RatePolicy(10 * time.Minute)
)

// RateKey is the Redis key prefix of end-offset snapshots.
type RateKey string

const RateKeyPrefix RateKey = "conn:rate:"

// OffsetSnapshot is the stored end offsets of one connection and the rates derived at that time.
type OffsetSnapshot struct {
	AtMs    int64              `json:"at"`
	Offsets map[string]int64   `json:"offsets"`
	Rates   map[string]float64 `json:"rates"`
}

// RateMath computes rates between two snapshots.
type RateMath struct{}

// Next returns the snapshot to store (nil = keep previous) and the rates to show.
func (RateMath) Next(prev *OffsetSnapshot, now time.Time, ends map[string]int64) (*OffsetSnapshot, map[string]*float64) {
	next := &OffsetSnapshot{AtMs: now.UnixMilli(), Offsets: ends, Rates: map[string]float64{}}
	if prev == nil {
		return next, map[string]*float64{}
	}
	elapsed := time.Duration(next.AtMs-prev.AtMs) * time.Millisecond
	if elapsed < time.Duration(RateMinInterval) {
		return nil, RateMath{}.pointers(prev.Rates)
	}
	for topic, end := range ends {
		before, ok := prev.Offsets[topic]
		if ok && end >= before {
			next.Rates[topic] = float64(end-before) / elapsed.Seconds()
		}
	}
	return next, RateMath{}.pointers(next.Rates)
}

func (RateMath) pointers(rates map[string]float64) map[string]*float64 {
	out := make(map[string]*float64, len(rates))
	for topic, r := range rates {
		v := r
		out[topic] = &v
	}
	return out
}

// RedisRateEstimator keeps the last snapshot per connection in Redis (TTL 10 min).
type RedisRateEstimator struct {
	redis redis.UniversalClient
	clock Clock
	math  RateMath
}

// NewRedisRateEstimator builds the estimator.
func NewRedisRateEstimator(client redis.UniversalClient, clock Clock) *RedisRateEstimator {
	if clock == nil {
		clock = time.Now
	}
	return &RedisRateEstimator{redis: client, clock: clock}
}

// Rates implements RateEstimator.
func (e *RedisRateEstimator) Rates(ctx context.Context, connID uuid.UUID, ends map[string]int64) (map[string]*float64, error) {
	key := string(RateKeyPrefix) + connID.String()
	prev, err := e.load(ctx, key)
	if err != nil {
		return nil, err
	}
	next, rates := e.math.Next(prev, e.clock(), ends)
	if next == nil {
		return rates, nil
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	return rates, e.redis.Set(ctx, key, raw, time.Duration(RateSnapshotTTL)).Err()
}

func (e *RedisRateEstimator) load(ctx context.Context, key string) (*OffsetSnapshot, error) {
	raw, err := e.redis.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var snap OffsetSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, nil
	}
	return &snap, nil
}
