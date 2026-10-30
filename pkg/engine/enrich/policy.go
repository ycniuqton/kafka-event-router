package enrich

import (
	"errors"
	"fmt"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Step error reasons of source lookups.
const (
	ReasonLookupFailed step.StepErrorReason = "lookup_failed"
	ReasonLookupMiss   step.StepErrorReason = "lookup_miss"
)

// ErrMiss is the cause of `onMiss: fail`.
var ErrMiss = errors.New("lookup source has no answer")

// LookupFailure is a lookup step error that declares its engine error type (lookup_unavailable / lookup_miss).
type LookupFailure struct {
	Type   engine.ErrorType
	Reason step.StepErrorReason
	Source string
	Cause  error
}

func (e *LookupFailure) Error() string {
	return fmt.Sprintf("%s (source %s): %v", e.Reason, e.Source, e.reported())
}

// reported is the cause shown to users: the dependency wrapper already names the source, so it is skipped.
func (e *LookupFailure) reported() error {
	if de, ok := e.Cause.(*engine.DependencyError); ok {
		return de.Cause
	}
	return e.Cause
}

func (e *LookupFailure) Unwrap() error { return e.Cause }

// EngineErrorType implements step.TypedError.
func (e *LookupFailure) EngineErrorType() engine.ErrorType { return e.Type }

// PolicyInput is what a miss or error policy acts on.
type PolicyInput struct {
	Event    engine.Event
	Mappings Mappings
	Source   string
	Cause    error
}

// MissPolicy decides the outcome when the source has no answer.
type MissPolicy interface {
	Apply(in PolicyInput) ([]engine.Event, error)
}

// ErrorPolicy decides the outcome when the source call failed after retries.
type ErrorPolicy interface {
	Apply(in PolicyInput) ([]engine.Event, error)
}

// SkipOutcome passes the event on unchanged.
type SkipOutcome struct{}

func (SkipOutcome) Apply(in PolicyInput) ([]engine.Event, error) { return step.One(in.Event), nil }

// DefaultOutcome writes a fixed value to every target.
type DefaultOutcome struct {
	Value any
}

func (p DefaultOutcome) Apply(in PolicyInput) ([]engine.Event, error) {
	ev := in.Event
	if err := in.Mappings.Fill(&ev, p.Value); err != nil {
		return nil, err
	}
	return step.One(ev), nil
}

// DropOutcome drops the event.
type DropOutcome struct{}

func (DropOutcome) Apply(PolicyInput) ([]engine.Event, error) { return nil, nil }

// FailMiss fails the event with lookup_miss (a data error: route error policy, DLQ).
type FailMiss struct{}

func (FailMiss) Apply(in PolicyInput) ([]engine.Event, error) {
	return nil, &LookupFailure{Type: engine.ErrLookupMiss, Reason: ReasonLookupMiss, Source: in.Source, Cause: ErrMiss}
}

// FailError fails the event: lookup_unavailable (engine.DependencyError contract) for transient causes,
// a plain step error otherwise.
type FailError struct{}

func (FailError) Apply(in PolicyInput) ([]engine.Event, error) {
	if (ctxhelpers.SourceErrors{}).IsTransient(in.Cause) {
		dep := &engine.DependencyError{Kind: engine.DependencyLookup, Name: in.Source, Cause: in.Cause}
		return nil, &LookupFailure{Type: engine.ErrLookupUnavailable, Reason: ReasonLookupFailed, Source: in.Source, Cause: dep}
	}
	return nil, &step.StepError{Reason: ReasonLookupFailed, Field: in.Source, Cause: in.Cause}
}

// PolicyManager builds miss and error policies from their document action (selection by registry).
type PolicyManager struct {
	miss  map[config.LookupMissAction]func(value any) MissPolicy
	error map[config.LookupErrorAction]func(value any) ErrorPolicy
}

// DefaultPolicies registers every documented action.
func DefaultPolicies() *PolicyManager {
	return &PolicyManager{
		miss: map[config.LookupMissAction]func(any) MissPolicy{
			config.LookupMissActionSkip:    func(any) MissPolicy { return SkipOutcome{} },
			config.LookupMissActionDefault: func(v any) MissPolicy { return DefaultOutcome{Value: v} },
			config.LookupMissActionDrop:    func(any) MissPolicy { return DropOutcome{} },
			config.LookupMissActionFail:    func(any) MissPolicy { return FailMiss{} },
		},
		error: map[config.LookupErrorAction]func(any) ErrorPolicy{
			config.LookupErrorActionFail:    func(any) ErrorPolicy { return FailError{} },
			config.LookupErrorActionSkip:    func(any) ErrorPolicy { return SkipOutcome{} },
			config.LookupErrorActionDefault: func(v any) ErrorPolicy { return DefaultOutcome{Value: v} },
		},
	}
}

// Miss returns the miss policy of raw (skip when absent).
func (m *PolicyManager) Miss(c *step.StepCompileCtx, ptr config.Pointer, raw *config.LookupMissPolicy) (MissPolicy, bool) {
	if raw == nil {
		return SkipOutcome{}, true
	}
	build, ok := m.miss[raw.Action]
	if !ok {
		c.Scope.Fail(config.IssueSchemaInvalid, ptr.Token(TokenAction), "onMiss action %q is not supported", raw.Action)
		return nil, false
	}
	return build(step.NumberNormalizer{}.Of(raw.Value)), true
}

// Error returns the error policy of raw (fail when absent).
func (m *PolicyManager) Error(c *step.StepCompileCtx, ptr config.Pointer, raw *config.LookupErrorPolicy) (ErrorPolicy, bool) {
	if raw == nil {
		return FailError{}, true
	}
	build, ok := m.error[raw.Action]
	if !ok {
		c.Scope.Fail(config.IssueSchemaInvalid, ptr.Token(TokenAction), "onError action %q is not supported", raw.Action)
		return nil, false
	}
	return build(step.NumberNormalizer{}.Of(raw.Value)), true
}
