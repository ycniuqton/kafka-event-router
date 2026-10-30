package failures

import (
	"errors"
	"fmt"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/sandbox"
)

// Failure is one record that could not be processed or delivered; RetryAttempts counts the delayed
// retry-topic attempts already used (written as router.retry_attempts when the record is parked).
type Failure struct {
	Source        engine.RawRecord
	Err           error
	Category      domain.ErrorCategory
	Attempts      int
	RetryAttempts int
}

// Step returns the failing step id from the engine error (empty when not a step failure).
func (f Failure) Step() string {
	var pe *engine.ProcessError
	if errors.As(f.Err, &pe) {
		return pe.StepID
	}
	return ""
}

// Branch returns the failing branch from the engine error.
func (f Failure) Branch() string {
	var pe *engine.ProcessError
	if errors.As(f.Err, &pe) {
		return string(pe.Branch)
	}
	return ""
}

// Stack returns the JavaScript stack trace of a sandbox failure ("" for other errors).
func (f Failure) Stack() string {
	if se, ok := sandbox.AsError(f.Err); ok {
		return se.Stack
	}
	return ""
}

// Message returns the error text (never the payload).
func (f Failure) Message() string {
	if f.Err == nil {
		return string(f.Category)
	}
	return f.Err.Error()
}

// HaltError stops the route without restart loop (error policy pause, destination permanent, source fatal).
type HaltError struct {
	Reason   domain.HaltReason
	Category domain.ErrorCategory
	Message  string
}

func (e *HaltError) Error() string {
	return fmt.Sprintf("route halted (%s): %s", e.Reason, e.Message)
}

// NewHalt builds a halt from a failure.
func NewHalt(reason domain.HaltReason, f Failure) *HaltError {
	return &HaltError{Reason: reason, Category: f.Category, Message: f.Message()}
}
