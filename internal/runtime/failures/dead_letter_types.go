package failures

import (
	"errors"
	"fmt"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/sandbox"
)

// PanicError is a panic recovered on the record path (a router bug, never the record's fault).
type PanicError struct {
	Value any
}

func (e *PanicError) Error() string { return fmt.Sprintf("panic: %v", e.Value) }

// DeadLetterTypeRule recognises one family of failures and names its DLQ error type.
type DeadLetterTypeRule interface {
	Type(f Failure) (record.ErrorType, bool)
}

// PanicRule: recovered panics are internal errors.
type PanicRule struct{}

func (PanicRule) Type(f Failure) (record.ErrorType, bool) {
	var pe *PanicError
	return record.ErrorInternal, errors.As(f.Err, &pe)
}

// DependencyRule: unavailable lookups and other dependencies (engine.DependencyError contract).
type DependencyRule struct{}

func (DependencyRule) Type(f Failure) (record.ErrorType, bool) {
	de, ok := engine.AsDependencyError(f.Err)
	if !ok {
		return "", false
	}
	if de.Kind == engine.DependencyLookup {
		return record.ErrorLookupUnavailable, true
	}
	return record.ErrorDependencyUnavailable, true
}

// sandboxKinds maps sandbox failure kinds that are never the script author's fault.
var sandboxKinds = map[sandbox.ErrorKind]record.ErrorType{
	sandbox.ErrorInternal: record.ErrorSandboxCrash,
	sandbox.ErrorHost:     record.ErrorSandboxCrash,
	sandbox.ErrorCompile:  record.ErrorConfig,
}

// SandboxRule: sandbox internal/host crashes and run-time compile failures.
type SandboxRule struct{}

func (SandboxRule) Type(f Failure) (record.ErrorType, bool) {
	se, ok := sandbox.AsError(f.Err)
	if !ok {
		return "", false
	}
	t, ok := sandboxKinds[se.Kind]
	return t, ok
}

// ConfigRule: compile errors reaching the record path are configuration bugs.
type ConfigRule struct{}

func (ConfigRule) Type(f Failure) (record.ErrorType, bool) {
	var ce *engine.CompileError
	return record.ErrorConfig, errors.As(f.Err, &ce)
}

// EncodeRule: an output for a branch the pipeline does not know is an encoder bug. Codec failures stay
// output_invalid: pipeline.Encode reports validation and codec failures alike, so they cannot be told apart.
type EncodeRule struct{}

func (EncodeRule) Type(f Failure) (record.ErrorType, bool) {
	return record.ErrorEncode, errors.Is(f.Err, compile.ErrUnknownBranch)
}

// defaultTypeRules are evaluated in order; the first match wins, else the category mapping applies.
var defaultTypeRules = []DeadLetterTypeRule{PanicRule{}, DependencyRule{}, SandboxRule{}, ConfigRule{}, EncodeRule{}}

// DeadLetterTypes names the DLQ error type of a failure (design 00001 §6.3), finer than the category.
type DeadLetterTypes struct {
	Rules []DeadLetterTypeRule
}

// Of returns the first matching rule's type, else the category's type.
func (t DeadLetterTypes) Of(f Failure) record.ErrorType {
	rules := t.Rules
	if rules == nil {
		rules = defaultTypeRules
	}
	for _, r := range rules {
		if et, ok := r.Type(f); ok {
			return et
		}
	}
	return ErrorTypes{}.Of(f.Category)
}
