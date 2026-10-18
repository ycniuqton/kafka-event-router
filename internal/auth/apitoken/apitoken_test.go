package apitoken

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/store"
)

type memRepo struct {
	mu        sync.Mutex
	tokens    map[uuid.UUID]Token
	touched   map[uuid.UUID]time.Time
	failTouch bool
}

func newMemRepo() *memRepo {
	return &memRepo{tokens: map[uuid.UUID]Token{}, touched: map[uuid.UUID]time.Time{}}
}

func (r *memRepo) Insert(_ context.Context, t Token) (Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t.ID, t.CreatedAt = uuid.New(), time.Now()
	r.tokens[t.ID] = t
	return t, nil
}

func (r *memRepo) ListActive(_ context.Context, userID uuid.UUID) ([]Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Token
	for _, t := range r.tokens {
		if t.UserID == userID && t.RevokedAt == nil {
			out = append(out, t)
		}
	}
	return out, nil
}

func (r *memRepo) CountActive(ctx context.Context, userID uuid.UUID) (int, error) {
	l, _ := r.ListActive(ctx, userID)
	return len(l), nil
}

func (r *memRepo) FindByHash(_ context.Context, hash []byte) (Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tokens {
		if string(t.Hash) == string(hash) {
			return t, nil
		}
	}
	return Token{}, store.ErrNotFound
}

func (r *memRepo) Revoke(_ context.Context, userID, id uuid.UUID, at time.Time) (Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tokens[id]
	if !ok || t.UserID != userID || t.RevokedAt != nil {
		return Token{}, store.ErrNotFound
	}
	t.RevokedAt = &at
	r.tokens[id] = t
	return t, nil
}

func (r *memRepo) TouchLastUsed(_ context.Context, used map[uuid.UUID]time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failTouch {
		return errors.New("db down")
	}
	for id, at := range used {
		r.touched[id] = at
	}
	return nil
}

type passTx struct{}

func (passTx) WithinTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
func (passTx) InTx(context.Context) bool                                          { return false }

type fakeUsers struct {
	user auth.User
	err  error
}

func (f fakeUsers) LoadUser(context.Context, uuid.UUID) (auth.User, []auth.TeamRole, error) {
	return f.user, []auth.TeamRole{{Role: auth.RoleEditor}}, f.err
}

type usageSink struct{ ids []uuid.UUID }

func (u *usageSink) Used(id uuid.UUID, _ time.Time) { u.ids = append(u.ids, id) }

func newManager(repo Repository, now time.Time) *Manager {
	return NewManager(ManagerDeps{Repo: repo, Tx: passTx{}, Scopes: auth.DefaultScopeCatalog(), Policy: DefaultPolicy(),
		Now: func() time.Time { return now }})
}

var owner = auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New()}

func TestCreateStoresOnlyTheHashAndShowsSecretOnce(t *testing.T) {
	repo := newMemRepo()
	m := newManager(repo, time.Now())
	created, err := m.Create(context.Background(), owner, CreateRequest{Name: " ci ", Scopes: []auth.TokenScope{auth.ScopeRead}})
	if err != nil {
		t.Fatal(err)
	}
	stored := repo.tokens[created.ID]
	if created.Token == "" || string(stored.Hash) != string(SecretCodec{}.Hash(created.Token)) || stored.Name != "ci" {
		t.Fatalf("unexpected stored token %+v", stored)
	}
	if want := time.Duration(DefaultTTLDays) * 24 * time.Hour; stored.ExpiresAt.Sub(stored.CreatedAt) < want-time.Minute {
		t.Fatalf("default expiry not applied: %v", stored.ExpiresAt)
	}
	list, _ := m.List(context.Background(), owner)
	if len(list) != 1 || list[0].Status != StatusActive {
		t.Fatalf("list = %+v", list)
	}
}

func TestCreateValidation(t *testing.T) {
	m := newManager(newMemRepo(), time.Now())
	tooLong := 400
	cases := map[string]CreateRequest{
		"name":   {Scopes: []auth.TokenScope{auth.ScopeRead}},
		"scopes": {Name: "x"},
		"scope":  {Name: "x", Scopes: []auth.TokenScope{"root"}},
		"expiry": {Name: "x", Scopes: []auth.TokenScope{auth.ScopeRead}, ExpiresInDays: &tooLong},
	}
	for name, req := range cases {
		var v *api.ValidationError
		if _, err := m.Create(context.Background(), owner, req); !errors.As(err, &v) {
			t.Errorf("%s: want validation error, got %v", name, err)
		}
	}
}

func TestTokensCannotManageTokens(t *testing.T) {
	m := newManager(newMemRepo(), time.Now())
	tokenPrincipal := auth.Principal{Kind: auth.PrincipalAPIToken, UserID: owner.UserID}
	if _, err := m.List(context.Background(), tokenPrincipal); !errors.Is(err, ErrInteractiveOnly) {
		t.Fatalf("want ErrInteractiveOnly, got %v", err)
	}
}

func TestRevokeOnlyOwnToken(t *testing.T) {
	m := newManager(newMemRepo(), time.Now())
	created, _ := m.Create(context.Background(), owner, CreateRequest{Name: "a", Scopes: []auth.TokenScope{auth.ScopeRead}})
	other := auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New()}
	if err := m.Revoke(context.Background(), other, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign revoke: %v", err)
	}
	if err := m.Revoke(context.Background(), owner, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Revoke(context.Background(), owner, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second revoke: %v", err)
	}
}

func TestAuthenticator(t *testing.T) {
	repo, now := newMemRepo(), time.Now()
	m := newManager(repo, now)
	created, _ := m.Create(context.Background(), owner, CreateRequest{Name: "a", Scopes: []auth.TokenScope{auth.ScopeWrite}})
	sink := &usageSink{}
	a := NewAuthenticator(repo, fakeUsers{user: auth.User{ID: owner.UserID}}, sink, func() time.Time { return now })
	cases := []struct {
		name, header string
		want         auth.Outcome
	}{
		{"none", "", auth.OutcomeNotApplicable},
		{"other bearer", "Bearer k8s-token", auth.OutcomeNotApplicable},
		{"unknown", "Bearer ert_nope", auth.OutcomeRejected},
		{"valid", "Bearer " + created.Token, auth.OutcomeMatched},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/api/v1/routes", nil)
		if c.header != "" {
			r.Header.Set("Authorization", c.header)
		}
		p, outcome, err := a.Authenticate(r)
		if err != nil || outcome != c.want {
			t.Fatalf("%s: outcome %s err %v", c.name, outcome, err)
		}
		if c.want == auth.OutcomeMatched && (p.Kind != auth.PrincipalAPIToken || p.TokenID != created.ID || p.Scopes[0] != auth.ScopeWrite) {
			t.Fatalf("principal %+v", p)
		}
	}
	if len(sink.ids) != 1 {
		t.Fatalf("usage recorded %d times", len(sink.ids))
	}
}

func TestAuthenticatorRejectsExpiredRevokedAndDisabled(t *testing.T) {
	repo, now := newMemRepo(), time.Now()
	created, _ := newManager(repo, now).Create(context.Background(), owner, CreateRequest{Name: "a", Scopes: []auth.TokenScope{auth.ScopeRead}})
	check := func(name string, a *Authenticator) {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer "+created.Token)
		if _, outcome, _ := a.Authenticate(r); outcome != auth.OutcomeRejected {
			t.Errorf("%s: want rejected, got %s", name, outcome)
		}
	}
	later := func() time.Time { return now.Add(91 * 24 * time.Hour) }
	check("expired", NewAuthenticator(repo, fakeUsers{user: auth.User{ID: owner.UserID}}, &usageSink{}, later))
	check("disabled", NewAuthenticator(repo, fakeUsers{user: auth.User{ID: owner.UserID, Disabled: true}}, &usageSink{}, nil))
	_ = newManager(repo, now).Revoke(context.Background(), owner, created.ID)
	check("revoked", NewAuthenticator(repo, fakeUsers{user: auth.User{ID: owner.UserID}}, &usageSink{}, nil))
}

func TestUsageBatcherKeepsLatestAndRetriesOnFailure(t *testing.T) {
	repo := newMemRepo()
	b := NewUsageBatcher(repo, DefaultFlushInterval, slog.New(slog.NewTextHandler(io.Discard, nil)))
	id, t0 := uuid.New(), time.Now()
	b.Used(id, t0.Add(time.Second))
	b.Used(id, t0)
	repo.failTouch = true
	b.Flush(context.Background())
	if len(repo.touched) != 0 {
		t.Fatal("failed flush must not persist")
	}
	repo.failTouch = false
	b.Flush(context.Background())
	if !repo.touched[id].Equal(t0.Add(time.Second)) {
		t.Fatalf("want latest time after retry, got %v", repo.touched[id])
	}
}
