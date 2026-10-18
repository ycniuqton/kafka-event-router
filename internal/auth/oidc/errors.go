// Package oidc implements the generic OpenID Connect login (authorization code + PKCE) as an auth.LoginFlow.
package oidc

import "github.com/eventrouter/router/internal/platform/problem"

// FailureCode is the reason shown on /login?error=<code> after a failed callback.
type FailureCode string

const (
	FailureState          FailureCode = "state"
	FailureIdPUnavailable FailureCode = "idp_unavailable"
	FailureInvalidToken   FailureCode = "invalid_token"
)

// LoginError is a classified callback failure carrying the code the UI displays.
type LoginError struct {
	Code   FailureCode
	Detail string
	Cause  error
}

func (e *LoginError) Error() string             { return "oidc: " + string(e.Code) + ": " + e.Detail }
func (e *LoginError) Unwrap() error             { return e.Cause }
func (e *LoginError) ProblemKind() problem.Kind { return problem.KindUnauthenticated }

func newLoginError(code FailureCode, detail string, cause error) *LoginError {
	return &LoginError{Code: code, Detail: detail, Cause: cause}
}

// ErrBadReturnTo rejects absolute or protocol-relative return_to values (open-redirect guard).
var ErrBadReturnTo = problem.NewError(problem.KindMalformedBody, "return_to must be a relative path such as /routes.").At("/return_to")
