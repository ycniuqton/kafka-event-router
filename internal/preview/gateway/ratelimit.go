package gateway

import (
	"context"
	"log/slog"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/ratelimit"
)

// RateKey namespaces the preview limiter keys (separate bucket from the global API limit).
type RateKey string

const RateKeyPrefix RateKey = "preview:"

// RateLog names the limiter's log lines.
type RateLog string

const LogPreviewLimiterDown RateLog = "preview.rate_limiter_unavailable"

// PreviewRateLimiter limits sample loads and runs per user (Redis GCRA shared by every API replica);
// a limiter outage fails open so preview keeps working.
type PreviewRateLimiter struct {
	limiter ratelimit.Limiter
	rule    ratelimit.Rule
	log     *slog.Logger
}

// NewPreviewRateLimiter builds the limiter over the platform limiter implementation.
func NewPreviewRateLimiter(l ratelimit.Limiter, rule ratelimit.Rule, log *slog.Logger) *PreviewRateLimiter {
	return &PreviewRateLimiter{limiter: l, rule: rule, log: log}
}

// Check returns a 429 problem (with Retry-After) when p exceeded the preview rate.
func (l *PreviewRateLimiter) Check(ctx context.Context, p auth.Principal) error {
	if l == nil {
		return nil
	}
	d, err := l.limiter.Allow(ctx, l.Key(p), l.rule)
	if err != nil {
		l.log.Warn(string(LogPreviewLimiterDown), string(logging.AttrError), err)
		return nil
	}
	if d.Allowed {
		return nil
	}
	return &ratelimit.LimitedError{RetryAfter: d.RetryAfter}
}

// Key identifies the caller: the API token, else the user.
func (l *PreviewRateLimiter) Key(p auth.Principal) string {
	if p.Kind == auth.PrincipalAPIToken {
		return string(RateKeyPrefix) + string(ratelimit.KeyToken) + p.TokenID.String()
	}
	return string(RateKeyPrefix) + string(ratelimit.KeyUser) + p.UserID.String()
}
