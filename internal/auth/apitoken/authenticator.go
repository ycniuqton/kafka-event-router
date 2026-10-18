package apitoken

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/store"
)

// HeaderValue names the parts of the Authorization header API tokens use.
type HeaderValue string

const (
	HeaderAuthorization HeaderValue = "Authorization"
	SchemeBearer        HeaderValue = "Bearer "
)

// UsageRecorder notes that a token authenticated a request (persisted later, in batches).
type UsageRecorder interface {
	Used(id uuid.UUID, at time.Time)
}

// Authenticator implements auth.Authenticator for `Authorization: Bearer ert_…` (mode api_token).
type Authenticator struct {
	repo       Repository
	principals auth.PrincipalSource
	usage      UsageRecorder
	codec      SecretCodec
	now        auth.Clock
}

// NewAuthenticator builds the authenticator.
func NewAuthenticator(repo Repository, principals auth.PrincipalSource, usage UsageRecorder, now auth.Clock) *Authenticator {
	if now == nil {
		now = time.Now
	}
	return &Authenticator{repo: repo, principals: principals, usage: usage, now: now}
}

// Kind implements auth.Authenticator.
func (a *Authenticator) Kind() auth.AuthenticatorKind {
	return auth.AuthenticatorAPIToken
}

// Authenticate resolves the token owner; unknown, revoked or expired tokens and disabled owners are rejected.
func (a *Authenticator) Authenticate(r *http.Request) (auth.Principal, auth.Outcome, error) {
	value := a.bearer(r)
	if !a.codec.Recognises(value) {
		return auth.Anonymous, auth.OutcomeNotApplicable, nil
	}
	token, err := a.repo.FindByHash(r.Context(), a.codec.Hash(value))
	if errors.Is(err, store.ErrNotFound) {
		return auth.Anonymous, auth.OutcomeRejected, nil
	}
	if err != nil {
		return auth.Anonymous, auth.OutcomeRejected, err
	}
	now := a.now()
	if !token.Usable(now) {
		return auth.Anonymous, auth.OutcomeRejected, nil
	}
	return a.principal(r.Context(), token, now)
}

func (a *Authenticator) principal(ctx context.Context, token Token, now time.Time) (auth.Principal, auth.Outcome, error) {
	user, roles, err := a.principals.LoadUser(ctx, token.UserID)
	if errors.Is(err, store.ErrNotFound) {
		return auth.Anonymous, auth.OutcomeRejected, nil
	}
	if err != nil {
		return auth.Anonymous, auth.OutcomeRejected, err
	}
	if user.Disabled {
		return auth.Anonymous, auth.OutcomeRejected, nil
	}
	a.usage.Used(token.ID, now)
	return auth.Principal{Kind: auth.PrincipalAPIToken, UserID: user.ID, Email: user.Email, DisplayName: user.DisplayName,
		Teams: roles, TokenID: token.ID, Scopes: token.Scopes}, auth.OutcomeMatched, nil
}

func (a *Authenticator) bearer(r *http.Request) string {
	h := r.Header.Get(string(HeaderAuthorization))
	if !strings.HasPrefix(h, string(SchemeBearer)) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, string(SchemeBearer)))
}

// FlushInterval is how often batched last-used times are written.
type FlushInterval time.Duration

const DefaultFlushInterval FlushInterval = FlushInterval(time.Minute)

// LogMessage names the usage batcher's log lines.
type LogMessage string

const LogFlushFailed LogMessage = "api token: last_used_at flush failed"

// UsageBatcher collects last-used times in memory and writes them every interval (and on shutdown).
type UsageBatcher struct {
	repo     Repository
	interval time.Duration
	log      *slog.Logger
	mu       sync.Mutex
	pending  map[uuid.UUID]time.Time
}

// NewUsageBatcher builds a batcher.
func NewUsageBatcher(repo Repository, interval FlushInterval, log *slog.Logger) *UsageBatcher {
	return &UsageBatcher{repo: repo, interval: time.Duration(interval), log: log, pending: map[uuid.UUID]time.Time{}}
}

// Used implements UsageRecorder.
func (b *UsageBatcher) Used(id uuid.UUID, at time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if prev, ok := b.pending[id]; !ok || at.After(prev) {
		b.pending[id] = at
	}
}

// Run flushes every interval until ctx ends, then flushes once more.
func (b *UsageBatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			b.Flush(context.WithoutCancel(ctx))
			return
		case <-ticker.C:
			b.Flush(ctx)
		}
	}
}

// Flush writes the pending batch; on failure the batch is merged back for the next attempt.
func (b *UsageBatcher) Flush(ctx context.Context) {
	b.mu.Lock()
	batch := b.pending
	b.pending = map[uuid.UUID]time.Time{}
	b.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	if err := b.repo.TouchLastUsed(ctx, batch); err != nil {
		b.log.Warn(string(LogFlushFailed), string(logging.AttrError), err)
		for id, at := range batch {
			b.Used(id, at)
		}
	}
}
