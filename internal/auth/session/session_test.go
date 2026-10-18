package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

type principals struct{ users map[uuid.UUID]auth.User }

func (p principals) LoadUser(_ context.Context, id uuid.UUID) (auth.User, []auth.TeamRole, error) {
	u, ok := p.users[id]
	if !ok {
		return auth.User{}, nil, store.ErrNotFound
	}
	return u, []auth.TeamRole{{TeamName: "default", Role: auth.RoleAdmin}}, nil
}

type fixture struct {
	mr    *miniredis.Miniredis
	store *RedisStore
	authn *CookieAuthenticator
	clock *clock
	user  auth.User
	ttls  TTLs
}

func newFixture(t *testing.T) *fixture {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := &clock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	ttls := TTLs{Idle: 8 * time.Hour, Absolute: 7 * 24 * time.Hour}
	user := auth.User{ID: uuid.New(), Email: "ada@example.com", DisplayName: "Ada"}
	st := NewRedisStore(rdb, ttls, c.Now)
	jar := auth.NewCookieJar(false, ttls.Absolute)
	return &fixture{mr: mr, store: st, clock: c, user: user, ttls: ttls,
		authn: NewCookieAuthenticator(st, principals{users: map[uuid.UUID]auth.User{user.ID: user}}, jar, ttls, c.Now)}
}

func (f *fixture) login(t *testing.T) auth.Session {
	s, err := f.store.Create(context.Background(), auth.Session{UserID: f.user.ID, CreatedAt: f.clock.now, LastSeenAt: f.clock.now})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *fixture) request(sessionID string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	if sessionID != "" {
		r.AddCookie(&http.Cookie{Name: string(auth.CookieSession), Value: sessionID})
	}
	return r
}

func TestCookieAuthenticator(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(f *fixture, s auth.Session) string
		outcome auth.Outcome
		errKind problem.Kind
	}{
		{"no cookie", func(*fixture, auth.Session) string { return "" }, auth.OutcomeNotApplicable, ""},
		{"valid session", func(_ *fixture, s auth.Session) string { return s.ID }, auth.OutcomeMatched, ""},
		{"unknown session", func(*fixture, auth.Session) string { return "nope" }, auth.OutcomeRejected, ""},
		{"idle expired", func(f *fixture, s auth.Session) string { f.mr.FastForward(9 * time.Hour); return s.ID }, auth.OutcomeRejected, ""},
		{"absolute expired", func(f *fixture, s auth.Session) string {
			f.clock.now = f.clock.now.Add(8 * 24 * time.Hour)
			return s.ID
		}, auth.OutcomeRejected, ""},
		{"user deleted", func(f *fixture, s auth.Session) string { f.user.ID = uuid.New(); return s.ID }, auth.OutcomeRejected, ""},
		{"redis down fails closed", func(f *fixture, s auth.Session) string { f.mr.Close(); return s.ID }, auth.OutcomeRejected, problem.KindUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			s := f.login(t)
			if tc.name == "user deleted" {
				f.authn.principals = principals{users: map[uuid.UUID]auth.User{}}
			}
			p, outcome, err := f.authn.Authenticate(f.request(tc.setup(f, s)))
			if outcome != tc.outcome {
				t.Fatalf("outcome = %s, err %v", outcome, err)
			}
			var c problem.Classified
			if tc.errKind != "" && (!errors.As(err, &c) || c.ProblemKind() != tc.errKind) {
				t.Fatalf("err = %v", err)
			}
			if outcome == auth.OutcomeMatched && (p.UserID != f.user.ID || p.CSRFToken != s.CSRFToken || p.SessionID != s.ID || !p.HasRole(auth.RoleAdmin)) {
				t.Fatalf("principal = %+v", p)
			}
		})
	}
}

func TestSessionTouchSlidesIdleTTL(t *testing.T) {
	f := newFixture(t)
	s := f.login(t)
	f.mr.FastForward(7 * time.Hour)
	f.clock.now = f.clock.now.Add(7 * time.Hour)
	if _, outcome, _ := f.authn.Authenticate(f.request(s.ID)); outcome != auth.OutcomeMatched {
		t.Fatal("session should still be valid")
	}
	f.mr.FastForward(7 * time.Hour)
	if _, outcome, _ := f.authn.Authenticate(f.request(s.ID)); outcome != auth.OutcomeMatched {
		t.Fatal("touch should have slid the idle TTL")
	}
}

func TestRevokeAndRevokeAll(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, b := f.login(t), f.login(t)
	if a.ID == b.ID || a.CSRFToken == "" {
		t.Fatal("ids must be unique and csrf set")
	}
	_ = f.store.Revoke(ctx, a.ID)
	if _, err := f.store.Get(ctx, a.ID); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("revoked: %v", err)
	}
	_ = f.store.RevokeAllForUser(ctx, f.user.ID)
	if _, err := f.store.Get(ctx, b.ID); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("revoke all: %v", err)
	}
}

// TestDisabledUserSessionsAreRevoked (PC-63): a disabled user's next request revokes every session.
func TestDisabledUserSessionsAreRevoked(t *testing.T) {
	f := newFixture(t)
	first, second := f.login(t), f.login(t)
	disabled := f.user
	disabled.Disabled = true
	f.authn = NewCookieAuthenticator(f.store, principals{users: map[uuid.UUID]auth.User{f.user.ID: disabled}},
		auth.NewCookieJar(false, f.ttls.Absolute), f.ttls, f.clock.Now)
	if _, outcome, err := f.authn.Authenticate(f.request(first.ID)); err != nil || outcome != auth.OutcomeRejected {
		t.Fatalf("disabled user: outcome %s err %v", outcome, err)
	}
	for _, id := range []string{first.ID, second.ID} {
		if _, err := f.store.Get(context.Background(), id); !errors.Is(err, auth.ErrSessionNotFound) {
			t.Fatalf("session %s survived: %v", id, err)
		}
	}
}
