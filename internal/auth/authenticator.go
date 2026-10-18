package auth

import (
	"net/http"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/problem"
)

// AuthMode is an authentication mode enabled by ROUTER_AUTH_MODES.
type AuthMode = config.AuthMode

// Outcome is the result of one authenticator looking at a request.
type Outcome string

const (
	OutcomeMatched       Outcome = "matched"
	OutcomeNotApplicable Outcome = "not_applicable"
	OutcomeRejected      Outcome = "rejected"
)

// AuthenticatorKind names a request authenticator implementation (several modes may share one).
type AuthenticatorKind string

const (
	AuthenticatorSession  AuthenticatorKind = "session_cookie"
	AuthenticatorAPIToken AuthenticatorKind = "api_token"
)

// Authenticator is the base of every request authenticator.
type Authenticator interface {
	Kind() AuthenticatorKind
	Authenticate(r *http.Request) (Principal, Outcome, error)
}

var (
	// ErrUnauthenticated means no valid credential was presented.
	ErrUnauthenticated = problem.NewError(problem.KindUnauthenticated, "You are not signed in or your session expired. Sign in and retry.")
	// ErrAuthUnavailable means the session store is down; requests fail closed.
	ErrAuthUnavailable = problem.NewError(problem.KindUnavailable, "Sign-in service is temporarily unavailable. Retry in a moment.")
	// ErrForbidden means the principal lacks the required permission.
	ErrForbidden = problem.NewError(problem.KindForbidden, "You don't have permission to do this. Ask a workspace admin for access.")
)
