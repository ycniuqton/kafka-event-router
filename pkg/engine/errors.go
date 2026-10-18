package engine

import (
	"fmt"

	"github.com/eventrouter/router/pkg/config"
)

// ErrorType classifies processing failures for the runtime's error policy.
type ErrorType string

const (
	ErrDecode          ErrorType = "decode"
	ErrDecodeTransient ErrorType = "decode_transient"
	ErrStep            ErrorType = "step"
	ErrStepTimeout     ErrorType = "step_timeout"
	ErrOutputInvalid   ErrorType = "output_invalid"
	ErrLimit           ErrorType = "limit"
)

// StepFilter is the StepID reported when a filter node fails.
const StepFilter = "filter"

// ProcessError is a typed failure of one record; Path is the JSON Pointer of the step or node.
type ProcessError struct {
	Type   ErrorType
	Branch BranchID
	StepID string
	Path   config.Pointer
	Cause  error
}

func (e *ProcessError) Error() string {
	if e.StepID != "" {
		return fmt.Sprintf("%s error in step %q: %v", e.Type, e.StepID, e.Cause)
	}
	return fmt.Sprintf("%s error: %v", e.Type, e.Cause)
}

func (e *ProcessError) Unwrap() error { return e.Cause }
