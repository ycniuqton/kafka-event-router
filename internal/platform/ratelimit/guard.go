package ratelimit

import (
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/problem"
)

// KeyKind prefixes a rate-limit key with what it identifies.
type KeyKind string

const (
	KeyUser  KeyKind = "user:"
	KeyToken KeyKind = "token:"
	KeyIP    KeyKind = "ip:"
)

// LogMessage names the guard's log lines.
type LogMessage string

const LogLimiterDown LogMessage = "rate limiter unavailable, request allowed"

// RateLimitedDetail is the user-facing message of a 429.
type RateLimitedDetail string

const DetailRateLimited RateLimitedDetail = "Too many requests. Slow down and retry after the time in Retry-After."

// LimitedError is the 429 problem carrying Retry-After.
type LimitedError struct {
	RetryAfter time.Duration
}

func (e *LimitedError) Error() string             { return string(DetailRateLimited) }
func (e *LimitedError) ProblemKind() problem.Kind { return problem.KindRateLimited }

// RetryAfterSeconds implements problem.Retryable (rounded up, at least 1).
func (e *LimitedError) RetryAfterSeconds() int {
	return max(1, int(math.Ceil(e.RetryAfter.Seconds())))
}

// Guard is the Manager business code and middleware talk to: it keys requests by principal and asks the limiter.
type Guard struct {
	limiter Limiter
	rule    Rule
	log     *slog.Logger
}

// NewGuard builds a guard over a limiter and rule.
func NewGuard(limiter Limiter, rule Rule, log *slog.Logger) *Guard {
	return &Guard{limiter: limiter, rule: rule, log: log}
}

// Check returns a LimitedError when the caller exceeded its rate; limiter outages fail open (logged).
func (g *Guard) Check(r *http.Request) error {
	d, err := g.limiter.Allow(r.Context(), g.Key(r), g.rule)
	if err != nil {
		g.log.Warn(string(LogLimiterDown), string(logging.AttrError), err)
		return nil
	}
	if d.Allowed {
		return nil
	}
	return &LimitedError{RetryAfter: d.RetryAfter}
}

// Key identifies the caller: token id, user id, else client IP.
func (g *Guard) Key(r *http.Request) string {
	p := auth.PrincipalFrom(r.Context())
	switch p.Kind {
	case auth.PrincipalAPIToken:
		return string(KeyToken) + p.TokenID.String()
	case auth.PrincipalUser:
		return string(KeyUser) + p.UserID.String()
	default:
		return string(KeyIP) + auth.ClientIP(r)
	}
}
