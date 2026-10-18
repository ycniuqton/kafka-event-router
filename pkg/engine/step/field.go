package step

import (
	"errors"

	"github.com/eventrouter/router/pkg/config"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/path"
)

// ErrMissing is the cause of field_missing step errors.
var ErrMissing = errors.New("field is missing")

// FieldRef is a parsed field path used by recipes.
type FieldRef struct {
	path.FieldPath
}

// Write sets v and wraps failures as write_failed step errors.
func (f FieldRef) Write(ev *engine.Event, v any) error {
	if err := f.Set(ev, v); err != nil {
		return &StepError{Reason: ReasonWriteFailed, Field: f.String(), Cause: err}
	}
	return nil
}

// Strictness decides what a missing field means for a recipe.
type Strictness struct {
	Strict bool
}

// Missing returns a field_missing error when strict, nil (no-op) otherwise.
func (s Strictness) Missing(f FieldRef) error {
	if s.Strict {
		return &StepError{Reason: ReasonFieldMissing, Field: f.String(), Cause: ErrMissing}
	}
	return nil
}

// Base holds the identity and strictness shared by recipe steps.
type Base struct {
	StepID   string
	StepKind config.StepKind
	Strictness
}

// NewBase builds the shared identity of a compiled recipe.
func NewBase(c *StepCompileCtx, raw config.Step) Base {
	return Base{StepID: c.ID, StepKind: raw.Kind, Strictness: Strictness{Strict: raw.IsStrict()}}
}

func (b Base) ID() string            { return b.StepID }
func (b Base) Kind() config.StepKind { return b.StepKind }

// One wraps a single output event.
func One(ev engine.Event) []engine.Event { return []engine.Event{ev} }
