package password

import (
	"math"
	"time"

	"github.com/eventrouter/router/internal/platform/problem"
)

// ProblemCode is the machine-readable `code` member of password problems.
type ProblemCode string

const (
	CodeInvalidCredentials     ProblemCode = "invalid_credentials"
	CodeLoginLocked            ProblemCode = "login_locked"
	CodeChangeRequired         ProblemCode = "password_change_required"
	CodeInvalidCurrentPassword ProblemCode = "invalid_current_password"
	CodeWeakPassword           ProblemCode = "weak_password"
	CodePasswordReused         ProblemCode = "password_reused"
)

// FieldPath names the JSON Pointer of the request members password problems point at.
type FieldPath string

const (
	PathEmail           FieldPath = "/email"
	PathPassword        FieldPath = "/password"
	PathCurrentPassword FieldPath = "/current_password"
	PathNewPassword     FieldPath = "/new_password"
)

// Error is a classified password problem with a reason code and an optional JSON Pointer.
type Error struct {
	Kind   problem.Kind
	Code   ProblemCode
	Detail string
	Path   FieldPath
}

func (e *Error) Error() string             { return e.Detail }
func (e *Error) ProblemKind() problem.Kind { return e.Kind }
func (e *Error) ProblemCode() string       { return string(e.Code) }
func (e *Error) ProblemPath() string       { return string(e.Path) }

// Is matches any Error with the same code (copies made by At stay comparable with errors.Is).
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// At returns a copy of e pointing at path (the same rule reported for another request member).
func (e *Error) At(path FieldPath) *Error {
	c := *e
	c.Path = path
	return &c
}

var (
	// ErrInvalidCredentials never tells whether the email exists.
	ErrInvalidCredentials = &Error{Kind: problem.KindUnauthenticated, Code: CodeInvalidCredentials,
		Detail: "The email or password is incorrect."}
	// ErrChangeRequired blocks every call until the first password change is done.
	ErrChangeRequired = &Error{Kind: problem.KindForbidden, Code: CodeChangeRequired,
		Detail: "Change your password before you continue."}
	// ErrInvalidCurrentPassword rejects a self-service change with a wrong current password.
	ErrInvalidCurrentPassword = &Error{Kind: problem.KindValidation, Code: CodeInvalidCurrentPassword,
		Detail: "The current password is incorrect.", Path: PathCurrentPassword}
	// ErrWeakPassword rejects passwords outside the policy.
	ErrWeakPassword = &Error{Kind: problem.KindValidation, Code: CodeWeakPassword,
		Detail: "Use a password of at least 12 and at most 256 characters.", Path: PathNewPassword}
	// ErrPasswordReused rejects a change to the same password.
	ErrPasswordReused = &Error{Kind: problem.KindValidation, Code: CodePasswordReused,
		Detail: "Choose a password different from the current one.", Path: PathNewPassword}
)

// LockedError is the 429 of a locked account or a client over its login-attempt rate.
type LockedError struct {
	RetryAfter time.Duration
}

func (e *LockedError) Error() string {
	return "Too many sign-in attempts. Wait and try again after the time in Retry-After."
}
func (e *LockedError) ProblemKind() problem.Kind { return problem.KindRateLimited }
func (e *LockedError) ProblemCode() string       { return string(CodeLoginLocked) }

// RetryAfterSeconds implements problem.Retryable (rounded up, at least 1).
func (e *LockedError) RetryAfterSeconds() int {
	return max(1, int(math.Ceil(e.RetryAfter.Seconds())))
}
