package auth

import (
	"crypto/subtle"
	"net/http"

	"github.com/eventrouter/router/internal/platform/problem"
)

// CSRFHeader is the header carrying the double-submit token.
type CSRFHeader string

const HeaderCSRFToken CSRFHeader = "X-CSRF-Token"

// ErrCSRF is returned when a cookie-authenticated mutation lacks a matching CSRF token.
var ErrCSRF = problem.NewError(problem.KindForbidden, "CSRF token missing or invalid. Reload the page and retry.")

// CSRFMiddleware enforces the double-submit token on cookie-authenticated unsafe requests.
type CSRFMiddleware struct {
	cookies *CookieJar
	safe    map[string]bool
}

// NewCSRFMiddleware builds the middleware.
func NewCSRFMiddleware(cookies *CookieJar) *CSRFMiddleware {
	return &CSRFMiddleware{cookies: cookies, safe: map[string]bool{
		http.MethodGet: true, http.MethodHead: true, http.MethodOptions: true,
	}}
}

// Check passes safe methods and non-session principals; otherwise header, cookie and session token must match.
func (m *CSRFMiddleware) Check(r *http.Request) error {
	p := PrincipalFrom(r.Context())
	if m.safe[r.Method] || p.SessionID == "" {
		return nil
	}
	header := r.Header.Get(string(HeaderCSRFToken))
	cookie := m.cookies.Value(r, CookieCSRF)
	if header == "" || !m.equal(header, cookie) || !m.equal(header, p.CSRFToken) {
		return ErrCSRF
	}
	return nil
}

func (m *CSRFMiddleware) equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
