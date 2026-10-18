package api

import (
	"net/http"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/requestid"
)

// PrincipalAuthenticator resolves the caller of a request (auth.Manager or a workload manager).
type PrincipalAuthenticator interface {
	Authenticate(r *http.Request) (auth.Principal, error)
}

// AuthenticateStage puts the principal in the context according to RouteSpec.Auth.
type AuthenticateStage struct {
	Authenticator PrincipalAuthenticator
	Responder     *Responder
}

// Wrap implements RouteMiddleware.
func (s AuthenticateStage) Wrap(spec RouteSpec, next http.Handler) http.Handler {
	policy := spec.policy()
	if policy == AuthNone {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Authenticator.Authenticate(r)
		if err != nil && policy == AuthRequired {
			s.Responder.Problem(w, r, err)
			return
		}
		if err != nil {
			p = auth.Anonymous
		}
		AnnotatePrincipal(r, p.UserID.String())
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	})
}

// CSRFStage enforces the double-submit token on cookie-authenticated mutations.
type CSRFStage struct {
	CSRF      *auth.CSRFMiddleware
	Responder *Responder
}

// Wrap implements RouteMiddleware.
func (s CSRFStage) Wrap(_ RouteSpec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.CSRF.Check(r); err != nil {
			s.Responder.Problem(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AuthorizeStage enforces RouteSpec.Permission through the Authorizer port.
type AuthorizeStage struct {
	Authorize *auth.AuthorizeMiddleware
	Responder *Responder
}

// Wrap implements RouteMiddleware.
func (s AuthorizeStage) Wrap(spec RouteSpec, next http.Handler) http.Handler {
	if spec.Permission.IsZero() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.Authorize.Check(r, spec.Permission, spec.Object); err != nil {
			s.Responder.Problem(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AuditStage opens an audit Scope on mutating routes.
type AuditStage struct {
	Audit *audit.Middleware
}

// Wrap implements RouteMiddleware.
func (s AuditStage) Wrap(spec RouteSpec, next http.Handler) http.Handler {
	if !spec.Mutating {
		return next
	}
	return s.Audit.Wrap(spec.AuditAction, next)
}

// AuditContext builds the audit request context (actor, ip, request id) from a request.
func AuditContext(r *http.Request) audit.RequestContext {
	return audit.RequestContext{
		Actor:     auth.PrincipalActor(auth.PrincipalFrom(r.Context())),
		IP:        auth.ClientIP(r),
		RequestID: requestid.From(r.Context()),
	}
}

// RequestGuard rejects a request with a classified error (rate limiter, token scopes, …).
type RequestGuard interface {
	Check(r *http.Request) error
}

// RateLimitStage rejects callers over their rate with 429 + Retry-After (after Authenticate: keyed by principal).
type RateLimitStage struct {
	Guard     RequestGuard
	Responder *Responder
}

// Wrap implements RouteMiddleware; routes without authentication are keyed by client IP.
func (s RateLimitStage) Wrap(_ RouteSpec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.Guard.Check(r); err != nil {
			s.Responder.Problem(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ScopeStage enforces API-token scopes against the route's access level (other principals pass).
type ScopeStage struct {
	Scopes    *auth.ScopeGuard
	Responder *Responder
}

// Wrap implements RouteMiddleware.
func (s ScopeStage) Wrap(spec RouteSpec, next http.Handler) http.Handler {
	if spec.policy() == AuthNone {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.Scopes.Check(r, spec.Permission); err != nil {
			s.Responder.Problem(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}
