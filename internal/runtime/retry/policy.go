// Package retry holds the runtime backoff policy shared by producers, DLQ writes and the
// control-plane client: jittered exponential delays from BackoffMin to BackoffMax.
package retry

import (
	"context"
	"math"
	"math/rand/v2"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
)

// Policy computes jittered exponential backoff delays (unbounded attempts for transient errors).
type Policy struct {
	Min    time.Duration
	Max    time.Duration
	Factor float64
	Jitter float64
}

// DefaultPolicy is 100 ms → 30 s, factor 2, ±20 % jitter.
func DefaultPolicy() Policy {
	return Policy{Min: domain.BackoffMin.D(), Max: domain.BackoffMax.D(), Factor: float64(domain.DefaultBackoffFactor), Jitter: float64(domain.DefaultJitter)}
}

// Delay returns the wait before retry number attempt (0-based), capped at Max.
func (p Policy) Delay(attempt int) time.Duration {
	base := float64(p.Min) * math.Pow(p.Factor, float64(max(attempt, 0)))
	base = math.Min(base, float64(p.Max))
	spread := base * p.Jitter
	d := time.Duration(base - spread + rand.Float64()*2*spread)
	return min(max(d, p.Min), p.Max)
}

// Wait sleeps for Delay(attempt) or returns the context error when cancelled first.
func (p Policy) Wait(ctx context.Context, attempt int) error {
	t := time.NewTimer(p.Delay(attempt))
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// BackoffFn adapts the policy to franz-go's RetryBackoffFn (1-based tries).
func (p Policy) BackoffFn() func(int) time.Duration {
	return func(tries int) time.Duration { return p.Delay(tries - 1) }
}
