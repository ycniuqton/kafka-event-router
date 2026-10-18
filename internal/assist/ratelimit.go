package assist

import (
	"context"
	"sync"
	"time"

	"github.com/eventrouter/router/internal/platform/ratelimit"
)

// RequestLimiter bounds assistant calls per user (design: 30/min, shared by replicas through Redis).
type RequestLimiter interface {
	Allow(ctx context.Context, key string) (bool, time.Duration)
}

// LimiterKeyPrefix namespaces the assistant bucket in the platform limiter.
type LimiterKeyPrefix string

const AssistBucket LimiterKeyPrefix = "assist:"

// SharedLimiter is the assistant bucket on platform-core's limiter (Redis GCRA); it fails open on errors.
type SharedLimiter struct {
	Limiter ratelimit.Limiter
	Rule    ratelimit.Rule
}

// NewSharedLimiter allows perMinute calls per user with a burst of the same size.
func NewSharedLimiter(l ratelimit.Limiter, perMinute int) SharedLimiter {
	return SharedLimiter{Limiter: l, Rule: ratelimit.Rule{PerMinute: perMinute, Burst: perMinute}}
}

// Allow implements RequestLimiter.
func (s SharedLimiter) Allow(ctx context.Context, key string) (bool, time.Duration) {
	d, err := s.Limiter.Allow(ctx, string(AssistBucket)+key, s.Rule)
	if err != nil {
		return true, 0
	}
	return d.Allowed, d.RetryAfter
}

// LimiterWindow is the length of one counting window.
type LimiterWindow time.Duration

const MinuteWindow LimiterWindow = LimiterWindow(time.Minute)

// WindowLimiter is a per-process fixed-window counter (used when no shared limiter is wired).
type WindowLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	now     func() time.Time
	windows map[string]windowCount
}

type windowCount struct {
	start time.Time
	count int
}

// NewWindowLimiter allows limit calls per key and window.
func NewWindowLimiter(limit int, window LimiterWindow, now func() time.Time) *WindowLimiter {
	return &WindowLimiter{limit: limit, window: time.Duration(window), now: now, windows: map[string]windowCount{}}
}

// Allow counts one call; when refused it returns how long until the window resets.
func (l *WindowLimiter) Allow(_ context.Context, key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	w := l.windows[key]
	if now.Sub(w.start) >= l.window {
		w = windowCount{start: now}
	}
	if w.count >= l.limit {
		return false, w.start.Add(l.window).Sub(now)
	}
	w.count++
	l.windows[key] = w
	l.prune(now)
	return true, 0
}

// prune drops expired windows so the map stays bounded by active users.
func (l *WindowLimiter) prune(now time.Time) {
	for k, w := range l.windows {
		if now.Sub(w.start) >= l.window {
			delete(l.windows, k)
		}
	}
}
