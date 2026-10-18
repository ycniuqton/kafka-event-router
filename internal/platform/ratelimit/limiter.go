// Package ratelimit limits API requests per principal (user, API token, or client IP for anonymous
// callers) with a Redis-backed GCRA token bucket shared by every API replica.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/platform/config"
)

// Rule is a sustained rate plus a burst allowance.
type Rule struct {
	PerMinute int
	Burst     int
}

// Decision is the outcome of one Allow call.
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
}

// Limiter is the base of every rate limiter implementation.
type Limiter interface {
	Allow(ctx context.Context, key string, rule Rule) (Decision, error)
}

// Factory builds a limiter from configuration.
type Factory func(cfg config.RateLimitConfig, rdb redis.UniversalClient) (Limiter, error)

// Registry selects the configured limiter (ROUTER_RATE_LIMITER).
type Registry struct {
	factories map[config.RateLimiterKind]Factory
}

// NewRegistry returns a registry with the redis and none limiters.
func NewRegistry() *Registry {
	r := &Registry{factories: map[config.RateLimiterKind]Factory{}}
	r.Register(config.RateLimiterRedis, func(_ config.RateLimitConfig, rdb redis.UniversalClient) (Limiter, error) {
		return NewRedisLimiter(rdb), nil
	})
	r.Register(config.RateLimiterNone, func(config.RateLimitConfig, redis.UniversalClient) (Limiter, error) {
		return Unlimited{}, nil
	})
	return r
}

// Register adds a factory.
func (r *Registry) Register(kind config.RateLimiterKind, f Factory) {
	r.factories[kind] = f
}

// Build instantiates the configured limiter.
func (r *Registry) Build(cfg config.RateLimitConfig, rdb redis.UniversalClient) (Limiter, error) {
	f, ok := r.factories[cfg.Limiter]
	if !ok {
		return nil, fmt.Errorf("ratelimit: no limiter registered for %q", cfg.Limiter)
	}
	return f(cfg, rdb)
}

// Unlimited allows every request (ROUTER_RATE_LIMITER=none).
type Unlimited struct{}

// Allow implements Limiter.
func (Unlimited) Allow(context.Context, string, Rule) (Decision, error) {
	return Decision{Allowed: true}, nil
}
