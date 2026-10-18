package auth_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/ratelimit"
	"github.com/eventrouter/router/internal/testkit"
)

// TestRateLimitOnRedis (PC-72): the GCRA script on a real Redis limits per principal and reports Retry-After.
func TestRateLimitOnRedis(t *testing.T) {
	ep := testkit.NewManager().Require(t, testkit.ServiceRedis)
	opts, err := redis.ParseURL(ep.URL)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { _ = rdb.Close() })
	guard := ratelimit.NewGuard(ratelimit.NewRedisLimiter(rdb), ratelimit.Rule{PerMinute: 30, Burst: 5}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := httptest.NewRequest("GET", "/api/v1/routes", nil)
	r = r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{Kind: auth.PrincipalAPIToken, TokenID: uuid.New()}))
	allowed := 0
	var limited *ratelimit.LimitedError
	for i := 0; i < 8; i++ {
		err := guard.Check(r)
		if err == nil {
			allowed++
			continue
		}
		if !errors.As(err, &limited) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if allowed != 5 || limited == nil || limited.RetryAfterSeconds() < 1 || limited.RetryAfterSeconds() > 2 {
		t.Fatalf("allowed=%d limited=%+v", allowed, limited)
	}
}
