// Package step compiles transform steps (recipes and js) and runs them as a chain
// where each step maps one event to zero (drop), one or many (split) events.
package step

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/compilectx"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/filter"
	"github.com/eventrouter/router/pkg/engine/formula"
)

// Group is the catalogue group of a step kind.
type Group string

const (
	GroupFields  Group = "fields"
	GroupValues  Group = "values"
	GroupPrivacy Group = "privacy"
	GroupTypes   Group = "types"
	GroupFlow    Group = "flow"
	GroupStruct  Group = "structure"
	GroupKafka   Group = "kafka"
	GroupCode    Group = "code"
	GroupLibrary Group = "library"
)

// StepIDPrefix prefixes generated ids of steps saved without one.
const StepIDPrefix = "step-"

// StepContext is the per-event state handed to steps.
type StepContext struct {
	Ctx     context.Context
	Now     time.Time
	Helpers ctxhelpers.Set
	Meta    engine.RouteMeta
	Branch  engine.BranchID
	logs    []string
}

// Log buffers user log lines of the current step.
func (sc *StepContext) Log(lines ...string) { sc.logs = append(sc.logs, lines...) }

// TakeLogs returns and clears the buffered lines.
func (sc *StepContext) TakeLogs() []string {
	l := sc.logs
	sc.logs = nil
	return l
}

// Step is one compiled transform step.
type Step interface {
	ID() string
	Kind() config.StepKind
	Apply(sc *StepContext, in engine.Event) ([]engine.Event, error)
}

// StepCompileCtx is the compile state of one step.
type StepCompileCtx struct {
	Scope     *compilectx.Scope
	Pointer   config.Pointer
	ID        string
	Nodes     *filter.NodeCompilerManager
	Operators *filter.OperatorManager
	Values    *ValueSourceManager
	Formulas  *formula.Compiler
	Steps     *StepCompilerManager
	Libraries LibraryStack
}

// Fail records an error issue under the step pointer.
func (c *StepCompileCtx) Fail(code config.IssueCode, member config.PointerToken, format string, args ...any) {
	c.Scope.Fail(code, c.Pointer.Token(member), format, args...)
}

// Path parses a field path member of the step.
func (c *StepCompileCtx) Path(raw *string, member config.PointerToken) (FieldRef, bool) {
	if raw == nil {
		c.Fail(config.IssueSchemaInvalid, member, "%s is required", member)
		return FieldRef{}, false
	}
	p, ok := c.Scope.Path(*raw, c.Pointer.Token(member))
	return FieldRef{FieldPath: p}, ok
}

// StepCompiler compiles one step kind.
type StepCompiler interface {
	Kind() config.StepKind
	Group() Group
	Compile(c *StepCompileCtx, raw config.Step) (Step, bool)
}

// StepCompilerManager selects step compilers by kind; business logic uses only this.
type StepCompilerManager struct {
	compilers map[config.StepKind]StepCompiler
	order     []config.StepKind
}

// NewStepCompilerManager registers the given compilers.
func NewStepCompilerManager(cs ...StepCompiler) *StepCompilerManager {
	m := &StepCompilerManager{compilers: map[config.StepKind]StepCompiler{}}
	for _, c := range cs {
		m.compilers[c.Kind()] = c
		m.order = append(m.order, c.Kind())
	}
	return m
}

// Compile dispatches by kind; unknown kinds become StepUnsupported issues.
func (m *StepCompilerManager) Compile(c *StepCompileCtx, raw config.Step) (Step, bool) {
	sc, ok := m.compilers[raw.Kind]
	if !ok {
		c.Fail(config.IssueStepUnsupported, config.TokenKind, "step kind %q is not supported", raw.Kind)
		return nil, false
	}
	return sc.Compile(c, raw)
}

// All returns the compilers in registration order (catalogue).
func (m *StepCompilerManager) All() []StepCompiler {
	out := make([]StepCompiler, 0, len(m.order))
	for _, k := range m.order {
		out = append(out, m.compilers[k])
	}
	return out
}

// IDFor returns the step id, generating one from the index when absent.
func IDFor(raw config.Step, index int) string {
	if raw.ID != nil {
		return *raw.ID
	}
	return StepIDPrefix + strconv.Itoa(index)
}

// StepErrorReason classifies runtime step failures.
type StepErrorReason string

const (
	ReasonFieldMissing  StepErrorReason = "field_missing"
	ReasonConvertFailed StepErrorReason = "convert_failed"
	ReasonWriteFailed   StepErrorReason = "write_failed"
	ReasonHelperFailed  StepErrorReason = "helper_failed"
	ReasonScriptFailed  StepErrorReason = "script_failed"
	ReasonScriptOutput  StepErrorReason = "script_output_invalid"
	ReasonFormulaFailed StepErrorReason = "formula_failed"
	ReasonTypeMismatch  StepErrorReason = "type_mismatch"
)

// StepError is a runtime failure of one step on one event.
type StepError struct {
	Reason StepErrorReason
	Field  string
	Cause  error
}

func (e *StepError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s on %s: %v", e.Reason, e.Field, e.Cause)
	}
	return fmt.Sprintf("%s: %v", e.Reason, e.Cause)
}

func (e *StepError) Unwrap() error { return e.Cause }
