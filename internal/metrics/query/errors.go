package query

import (
	"errors"

	"github.com/eventrouter/router/internal/platform/problem"
)

// ProblemCode is the machine-readable reason of a metrics-store failure.
type ProblemCode string

const CodeMetricsUnavailable ProblemCode = "metrics_unavailable"

// UnavailableDetail is shown to users when the metrics store cannot answer.
type UnavailableDetail string

const DetailUnavailable UnavailableDetail = "Metrics are temporarily unavailable. Counts from the control plane still work; retry in a moment."

// UnavailableError reports a metrics store that is down, slow or answered garbage (503).
type UnavailableError struct {
	Cause error
}

func (e *UnavailableError) Error() string {
	if e.Cause == nil {
		return string(CodeMetricsUnavailable)
	}
	return string(CodeMetricsUnavailable) + ": " + e.Cause.Error()
}

func (e *UnavailableError) Unwrap() error             { return e.Cause }
func (e *UnavailableError) ProblemKind() problem.Kind { return problem.KindUnavailable }
func (e *UnavailableError) ProblemCode() string       { return string(CodeMetricsUnavailable) }

// Unavailable wraps cause as a metrics-store failure.
func Unavailable(cause error) error {
	return &UnavailableError{Cause: cause}
}

// IsUnavailable reports whether err is a metrics-store failure.
func IsUnavailable(err error) bool {
	var u *UnavailableError
	return errors.As(err, &u)
}

// ErrUnknownBackend reports a ROUTER_METRICS_BACKEND without a registered factory.
type ErrUnknownBackend struct {
	Kind BackendKind
}

func (e ErrUnknownBackend) Error() string {
	return "metrics: no querier registered for backend " + string(e.Kind)
}
