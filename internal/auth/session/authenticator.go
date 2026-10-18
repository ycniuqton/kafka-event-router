package session

import (
	"errors"
	"net/http"
	"time"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/store"
)

// TouchInterval limits how often a session's last-seen time is written.
type TouchInterval time.Duration

const DefaultTouchInterval TouchInterval = TouchInterval(time.Minute)

// CookieAuthenticator authenticates requests by the er_session cookie (modes oidc and dev).
type CookieAuthenticator struct {
	sessions   auth.SessionStore
	principals auth.PrincipalSource
	cookies    *auth.CookieJar
	ttls       TTLs
	now        auth.Clock
}

// NewCookieAuthenticator builds the authenticator.
func NewCookieAuthenticator(sessions auth.SessionStore, principals auth.PrincipalSource, cookies *auth.CookieJar, ttls TTLs, now auth.Clock) *CookieAuthenticator {
	if now == nil {
		now = time.Now
	}
	return &CookieAuthenticator{sessions: sessions, principals: principals, cookies: cookies, ttls: ttls, now: now}
}

// Kind implements auth.Authenticator.
func (a *CookieAuthenticator) Kind() auth.AuthenticatorKind {
	return auth.AuthenticatorSession
}

// Authenticate resolves the session principal; Redis failures fail closed with 503.
func (a *CookieAuthenticator) Authenticate(r *http.Request) (auth.Principal, auth.Outcome, error) {
	id := a.cookies.Value(r, auth.CookieSession)
	if id == "" {
		return auth.Anonymous, auth.OutcomeNotApplicable, nil
	}
	sess, err := a.sessions.Get(r.Context(), id)
	if errors.Is(err, auth.ErrSessionNotFound) {
		return auth.Anonymous, auth.OutcomeRejected, nil
	}
	if err != nil {
		return auth.Anonymous, auth.OutcomeRejected, err
	}
	if a.now().Sub(sess.CreatedAt) > a.ttls.Absolute {
		_ = a.sessions.Revoke(r.Context(), id)
		return auth.Anonymous, auth.OutcomeRejected, nil
	}
	return a.principal(r, sess)
}

func (a *CookieAuthenticator) principal(r *http.Request, sess auth.Session) (auth.Principal, auth.Outcome, error) {
	user, roles, err := a.principals.LoadUser(r.Context(), sess.UserID)
	if errors.Is(err, store.ErrNotFound) {
		_ = a.sessions.Revoke(r.Context(), sess.ID)
		return auth.Anonymous, auth.OutcomeRejected, nil
	}
	if err != nil {
		return auth.Anonymous, auth.OutcomeRejected, err
	}
	if user.Disabled {
		return auth.Anonymous, auth.OutcomeRejected, a.sessions.RevokeAllForUser(r.Context(), user.ID)
	}
	if a.now().Sub(sess.LastSeenAt) >= time.Duration(DefaultTouchInterval) {
		if err := a.sessions.Touch(r.Context(), sess); err != nil {
			return auth.Anonymous, auth.OutcomeRejected, err
		}
	}
	return auth.Principal{
		Kind: auth.PrincipalUser, UserID: user.ID, Email: user.Email, DisplayName: user.DisplayName,
		Teams: roles, SessionID: sess.ID, CSRFToken: sess.CSRFToken,
	}, auth.OutcomeMatched, nil
}
