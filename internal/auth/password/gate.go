package password

import (
	"net/http"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
)

// RouteKeys is a set of "METHOD /pattern" route keys.
type RouteKeys []string

// ChangeAllowed are the routes a user with a pending forced change may still call.
var ChangeAllowed = RouteKeys{"GET /api/v1/auth/me", "POST /api/v1/auth/password", "POST /auth/logout", "GET /auth/config"}

// GateStage rejects every other route with 403 password_change_required while the principal must change its password.
type GateStage struct {
	Allowed   RouteKeys
	Responder *api.Responder
}

// Wrap implements api.RouteMiddleware (public and allow-listed routes pass untouched).
func (s GateStage) Wrap(spec api.RouteSpec, next http.Handler) http.Handler {
	if spec.Auth == api.AuthNone || s.allows(spec) {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.PrincipalFrom(r.Context()).MustChangePassword {
			s.Responder.Problem(w, r, ErrChangeRequired)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s GateStage) allows(spec api.RouteSpec) bool {
	key := spec.Method + " " + spec.Pattern
	for _, allowed := range s.Allowed {
		if allowed == key {
			return true
		}
	}
	return false
}
