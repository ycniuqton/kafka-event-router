package sandbox

import (
	"errors"
	"fmt"
)

// Error is a typed step error raised by a check or a call.
type Error struct {
	Kind      ErrorKind `json:"kind"`
	Message   string    `json:"message"`
	Line      int       `json:"line,omitempty"`
	Column    int       `json:"column,omitempty"`
	Stack     string    `json:"stack,omitempty"`
	Retryable bool      `json:"retryable"`
}

// Error renders "kind: message (line:col)".
func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s: %s (%d:%d)", e.Kind, e.Message, e.Line, e.Column)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

// AsError extracts a sandbox *Error from err.
func AsError(err error) (*Error, bool) {
	var se *Error
	ok := errors.As(err, &se)
	return se, ok
}

// Diagnostic is a validation issue produced by Check or limit resolution.
type Diagnostic struct {
	Severity Severity `json:"severity"`
	Path     string   `json:"path,omitempty"`
	Message  string   `json:"message"`
	Line     int      `json:"line,omitempty"`
	Column   int      `json:"column,omitempty"`
}

// DiagnosticFromError converts a compile-time Error into an error Diagnostic at path.
func DiagnosticFromError(path string, e *Error) Diagnostic {
	return Diagnostic{Severity: SeverityError, Path: path, Message: e.Message, Line: e.Line, Column: e.Column}
}

// Message is a user-facing message template owned by the sandbox domain.
type Message string

const (
	MsgLimitClamped        Message = "requested %d exceeds the operator maximum; clamped to %d"
	MsgSourceTooLarge      Message = "source is %d bytes; the maximum is %d bytes"
	MsgTransformMissing    Message = "transform(event, ctx) is not defined"
	MsgPoolExhausted       Message = "no sandbox instance became free before the deadline"
	MsgPoolClosed          Message = "sandbox for this route version is closed"
	MsgTimeout             Message = "execution exceeded the %d ms time limit"
	MsgStackOverflow       Message = "maximum call stack size exceeded"
	MsgOutputTooLarge      Message = "output is %d bytes; the limit is %d bytes"
	MsgInvalidOutput       Message = "transform must return an object, null or an array of objects"
	MsgUnknownStep         Message = "no JS module for step %q"
	MsgUnknownEngine       Message = "no sandbox engine registered for %q"
	MsgInstanceTrapped     Message = "sandbox instance failed: %v"
	MsgOutOfMemory         Message = "memory limit exceeded"
	MsgEmptyPredicate      Message = "expression is empty"
	MsgUnsupportedEntry    Message = "unsupported entry kind %q"
	MsgUnknownProgramCache Message = "no program cache registered for %q"
	MsgDeterminismStale    Message = "recorded under determinism version %d; the sandbox runs version %d, re-record the expectation"
)

// Format fills the template.
func (m Message) Format(args ...any) string {
	return fmt.Sprintf(string(m), args...)
}

// ErrorPolicy says how the platform reacts to one error kind.
type ErrorPolicy struct {
	Retryable bool
	Action    PoolAction
}

// ErrorClassifier maps error kinds to retry and pool decisions (SysDesign §8.3).
type ErrorClassifier struct {
	policies map[ErrorKind]ErrorPolicy
}

// NewErrorClassifier returns the classification table of design §8.
func NewErrorClassifier() ErrorClassifier {
	return ErrorClassifier{policies: map[ErrorKind]ErrorPolicy{
		ErrorCompile:        {Retryable: false, Action: PoolRelease},
		ErrorRuntime:        {Retryable: false, Action: PoolRelease},
		ErrorTimeout:        {Retryable: true, Action: PoolDiscard},
		ErrorMemory:         {Retryable: false, Action: PoolDiscard},
		ErrorStackOverflow:  {Retryable: false, Action: PoolDiscard},
		ErrorOutputTooLarge: {Retryable: false, Action: PoolRelease},
		ErrorInvalidOutput:  {Retryable: false, Action: PoolRelease},
		ErrorHost:           {Retryable: false, Action: PoolRelease},
		ErrorInternal:       {Retryable: false, Action: PoolDiscard},
		ErrorDeterminism:    {Retryable: false, Action: PoolRelease},
	}}
}

// Policy returns the policy of kind; unknown kinds are treated as internal.
func (c ErrorClassifier) Policy(kind ErrorKind) ErrorPolicy {
	if p, ok := c.policies[kind]; ok {
		return p
	}
	return c.policies[ErrorInternal]
}

// New builds a classified Error.
func (c ErrorClassifier) New(kind ErrorKind, msg string) *Error {
	return &Error{Kind: kind, Message: msg, Retryable: c.Policy(kind).Retryable}
}

// Classify normalises any error into a classified *Error (non-sandbox errors become internal).
func (c ErrorClassifier) Classify(err error) *Error {
	if se, ok := AsError(err); ok {
		return se
	}
	return c.New(ErrorInternal, err.Error())
}
