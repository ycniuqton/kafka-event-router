package auth

import (
	"net/http"
	"time"
)

// CookieName names the cookies set by the auth package.
type CookieName string

const (
	CookieSession   CookieName = "er_session"
	CookieCSRF      CookieName = "er_csrf"
	CookieOIDCState CookieName = "er_oidc_state"
)

// CookiePath is the path every auth cookie is scoped to.
type CookiePath string

const CookieRootPath CookiePath = "/"

// CookieJar writes and reads the session and CSRF cookies (HttpOnly, Secure per config, SameSite=Lax).
type CookieJar struct {
	secure bool
	maxAge time.Duration
}

// NewCookieJar builds a jar; secure follows ROUTER_COOKIE_SECURE (off by default only in local).
func NewCookieJar(secure bool, maxAge time.Duration) *CookieJar {
	return &CookieJar{secure: secure, maxAge: maxAge}
}

// SetSession writes er_session (HttpOnly) and er_csrf (readable by JS).
func (j *CookieJar) SetSession(w http.ResponseWriter, s Session) {
	http.SetCookie(w, j.cookie(CookieSession, s.ID, true, j.maxAge))
	http.SetCookie(w, j.cookie(CookieCSRF, s.CSRFToken, false, j.maxAge))
}

// Clear expires both session cookies.
func (j *CookieJar) Clear(w http.ResponseWriter) {
	http.SetCookie(w, j.cookie(CookieSession, "", true, -time.Second))
	http.SetCookie(w, j.cookie(CookieCSRF, "", false, -time.Second))
}

// SetShortLived writes an HttpOnly cookie with a custom lifetime (e.g. OIDC state).
func (j *CookieJar) SetShortLived(w http.ResponseWriter, name CookieName, value string, ttl time.Duration) {
	http.SetCookie(w, j.cookie(name, value, true, ttl))
}

// Value returns a cookie's value, or "".
func (j *CookieJar) Value(r *http.Request, name CookieName) string {
	c, err := r.Cookie(string(name))
	if err != nil {
		return ""
	}
	return c.Value
}

func (j *CookieJar) cookie(name CookieName, value string, httpOnly bool, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     string(name),
		Value:    value,
		Path:     string(CookieRootPath),
		HttpOnly: httpOnly,
		Secure:   j.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl / time.Second),
	}
}
