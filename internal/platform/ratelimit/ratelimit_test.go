package ratelimit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/problem"
)

func newGuard(t *testing.T, rule Rule) (*Guard, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	limiter, err := NewRegistry().Build(config.RateLimitConfig{Limiter: config.RateLimiterRedis}, redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	if err != nil {
		t.Fatal(err)
	}
	return NewGuard(limiter, rule, slog.New(slog.NewTextHandler(io.Discard, nil))), mr
}

func TestGuardLimitsPerPrincipalWithRetryAfter(t *testing.T) {
	g, _ := newGuard(t, Rule{PerMinute: 60, Burst: 3})
	alice := auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New()}
	bob := auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New()}
	r := httptest.NewRequest("GET", "/api/v1/routes", nil)
	ra := r.WithContext(auth.WithPrincipal(r.Context(), alice))
	for i := 0; i < 3; i++ {
		if err := g.Check(ra); err != nil {
			t.Fatalf("request %d limited: %v", i, err)
		}
	}
	err := g.Check(ra)
	var limited *LimitedError
	if !errors.As(err, &limited) || limited.RetryAfterSeconds() < 1 {
		t.Fatalf("want 429 with retry-after, got %v", err)
	}
	d := problem.NewCatalog().Resolve(err)
	if d.Status != 429 || d.RetryAfter < 1 {
		t.Fatalf("problem = %+v", d)
	}
	if err := g.Check(r.WithContext(auth.WithPrincipal(r.Context(), bob))); err != nil {
		t.Fatalf("other principal must have its own bucket: %v", err)
	}
}

func TestGuardFailsOpenWhenRedisIsDown(t *testing.T) {
	g, mr := newGuard(t, Rule{PerMinute: 1, Burst: 1})
	mr.Close()
	if err := g.Check(httptest.NewRequest("GET", "/", nil)); err != nil {
		t.Fatalf("limiter outage must not block: %v", err)
	}
}

func TestKeys(t *testing.T) {
	g := NewGuard(Unlimited{}, Rule{}, nil)
	r := httptest.NewRequest("GET", "/", nil)
	tok := uuid.New()
	if k := g.Key(r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{Kind: auth.PrincipalAPIToken, TokenID: tok}))); k != "token:"+tok.String() {
		t.Fatal(k)
	}
	if k := g.Key(r); k != "ip:192.0.2.1" {
		t.Fatal(k)
	}
	if d, _ := (Unlimited{}).Allow(context.Background(), "k", Rule{}); !d.Allowed {
		t.Fatal("unlimited must allow")
	}
}
