package api

import (
	"fmt"

	"github.com/eventrouter/router/internal/platform/problem"
)

// ValidationError is the domain error every feature returns for invalid input → 422 with errors[].
type ValidationError struct {
	Errors []problem.FieldError
}

// NewValidationError builds an empty validation error to add field errors to.
func NewValidationError() *ValidationError {
	return &ValidationError{}
}

// Add appends a field error at a JSON Pointer path.
func (v *ValidationError) Add(path, code, message string) *ValidationError {
	v.Errors = append(v.Errors, problem.FieldError{Path: path, Code: code, Message: message})
	return v
}

// OrNil returns v when it holds errors, else nil (so callers can `return v.OrNil()`).
func (v *ValidationError) OrNil() error {
	if len(v.Errors) == 0 {
		return nil
	}
	return v
}

func (v *ValidationError) Error() string {
	return fmt.Sprintf("%d fields need attention.", len(v.Errors))
}

// ProblemKind implements problem.Classified.
func (v *ValidationError) ProblemKind() problem.Kind { return problem.KindValidation }

// FieldErrors implements problem.Itemised.
func (v *ValidationError) FieldErrors() []problem.FieldError { return v.Errors }
