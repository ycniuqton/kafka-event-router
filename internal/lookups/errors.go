package lookups

import (
	"github.com/eventrouter/router/internal/platform/problem"
)

// ProblemType names the lookup-source problem types (RFC 7807 `type`).
type ProblemType string

const ProblemSourceInUse ProblemType = "https://eventrouter.dev/problems/lookup-source-in-use"

// ErrorDetail holds user-facing problem details.
type ErrorDetail string

const (
	DetailInUse             ErrorDetail = "This lookup source is used by routes. Point their lookup steps to another source or archive the routes, then delete again."
	DetailSecretUnavailable ErrorDetail = "The stored credentials of this lookup source could not be read. Replace the secret and retry."
)

// InUseError blocks deleting a source that routes still use.
type InUseError struct {
	Routes []RouteRef
}

func (e *InUseError) Error() string               { return string(DetailInUse) }
func (e *InUseError) ProblemKind() problem.Kind   { return problem.KindConflict }
func (e *InUseError) ProblemTypeURI() ProblemType { return ProblemSourceInUse }

// ErrSecretUnavailable is returned when a stored secret cannot be decrypted (500, no detail leaked).
var ErrSecretUnavailable = problem.NewError(problem.KindInternal, string(DetailSecretUnavailable))
