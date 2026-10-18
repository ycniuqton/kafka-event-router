package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/eventrouter/router/internal/store"
)

// Clock returns the current time (replaceable in tests).
type Clock func() time.Time

// LoginManager drives interactive logins: flow → user upsert → login hook → session → cookies → audit.
type LoginManager struct {
	flows    map[AuthMode]LoginFlow
	users    UserProvisioner
	hook     LoginHook
	sessions SessionStore
	cookies  *CookieJar
	tx       store.TxManager
	auditor  *LoginAuditor
	tracker  LoginTracker
	now      Clock
}

// LoginDeps groups the collaborators of a LoginManager.
type LoginDeps struct {
	Users    UserProvisioner
	Hook     LoginHook
	Sessions SessionStore
	Cookies  *CookieJar
	Tx       store.TxManager
	Auditor  *LoginAuditor
	Tracker  LoginTracker
	Now      Clock
}

// NewLoginManager builds a manager over the configured flows.
func NewLoginManager(deps LoginDeps, flows ...LoginFlow) *LoginManager {
	m := &LoginManager{flows: map[AuthMode]LoginFlow{}, users: deps.Users, hook: deps.Hook,
		sessions: deps.Sessions, cookies: deps.Cookies, tx: deps.Tx, auditor: deps.Auditor, tracker: deps.Tracker, now: deps.Now}
	if m.tracker == nil {
		m.tracker = NoLoginTracking{}
	}
	if m.now == nil {
		m.now = time.Now
	}
	for _, f := range flows {
		m.flows[f.Mode()] = f
	}
	return m
}

// SetHook replaces the login hook (access-control's Bootstrapper replaces the default).
func (m *LoginManager) SetHook(h LoginHook) {
	m.hook = h
}

// Hook returns the current login hook (decorators such as the OIDC claim mapping wrap it).
func (m *LoginManager) Hook() LoginHook {
	return m.hook
}

// Modes lists the enabled interactive login modes.
func (m *LoginManager) Modes() []AuthMode {
	modes := make([]AuthMode, 0, len(m.flows))
	for mode := range m.flows {
		modes = append(modes, mode)
	}
	return modes
}

// Begin starts a login flow (e.g. redirect to the IdP).
func (m *LoginManager) Begin(mode AuthMode, w http.ResponseWriter, r *http.Request) error {
	flow, ok := m.flows[mode]
	if !ok {
		return ErrLoginModeDisabled
	}
	return flow.Begin(w, r)
}

// Complete finishes a login and returns the identity (with ReturnTo) after the session cookies are set.
func (m *LoginManager) Complete(mode AuthMode, w http.ResponseWriter, r *http.Request) (Identity, error) {
	flow, ok := m.flows[mode]
	if !ok {
		return Identity{}, ErrLoginModeDisabled
	}
	id, err := flow.Complete(w, r)
	if err != nil {
		m.auditor.LoginFailed(r, mode)
		return Identity{}, err
	}
	user, err := m.provision(r.Context(), id)
	if errors.Is(err, ErrUserDisabled) {
		m.auditor.LoginFailed(r, mode)
	}
	if err != nil {
		return Identity{}, err
	}
	sess, err := m.sessions.Create(r.Context(), m.newSession(r, user, mode))
	if err != nil {
		return Identity{}, err
	}
	m.cookies.SetSession(w, sess)
	m.auditor.LoggedIn(r, user, mode)
	return id, nil
}

// Logout revokes the current session and clears its cookies.
func (m *LoginManager) Logout(w http.ResponseWriter, r *http.Request) error {
	p := PrincipalFrom(r.Context())
	if p.SessionID != "" {
		if err := m.sessions.Revoke(r.Context(), p.SessionID); err != nil {
			return err
		}
	}
	m.cookies.Clear(w)
	m.auditor.LoggedOut(r, p)
	return nil
}

func (m *LoginManager) provision(ctx context.Context, id Identity) (User, error) {
	var user User
	err := m.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if user, err = m.users.Upsert(ctx, id); err != nil {
			return err
		}
		if user.Disabled {
			return ErrUserDisabled
		}
		if err = m.tracker.MarkLogin(ctx, user.ID, m.now()); err != nil {
			return err
		}
		return m.hook.OnLogin(ctx, user, id)
	})
	return user, err
}

func (m *LoginManager) newSession(r *http.Request, user User, mode AuthMode) Session {
	now := m.now()
	return Session{UserID: user.ID, Mode: mode, IP: ClientIP(r), UserAgent: r.UserAgent(), CreatedAt: now, LastSeenAt: now}
}
