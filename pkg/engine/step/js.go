package step

import (
	"errors"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/script"
)

// ErrScriptOutput is the cause when a js step returns something other than event, array or null.
var ErrScriptOutput = errors.New("transform must return an event object, an array of events or null")

// JSCompiler compiles `js` steps into sandbox transform modules.
type JSCompiler struct{}

func (JSCompiler) Kind() config.StepKind { return config.StepKindJS }
func (JSCompiler) Group() Group          { return GroupCode }

func (JSCompiler) Compile(c *StepCompileCtx, raw config.Step) (Step, bool) {
	if raw.Code == nil {
		c.Fail(config.IssueSchemaInvalid, config.TokenCode, "js step requires code")
		return nil, false
	}
	ptr := c.Pointer.Token(config.TokenCode)
	mod, ok := c.Scope.Script(script.Source{Code: *raw.Code, Entry: script.EntryTransform, Origin: string(ptr)}, ptr)
	if !ok {
		return nil, false
	}
	return &jsStep{id: c.ID, module: mod}, true
}

// jsStep calls transform(event, ctx): null drops, an array splits, an object replaces.
type jsStep struct {
	id     string
	module script.Module
}

func (s *jsStep) ID() string            { return s.id }
func (s *jsStep) Kind() config.StepKind { return config.StepKindJS }

func (s *jsStep) Apply(sc *StepContext, in engine.Event) ([]engine.Event, error) {
	res, err := s.module.Call(sc.Ctx, script.Invocation{Event: in.Snapshot(), Now: sc.Now})
	sc.Log(res.Logs...)
	if err != nil {
		return nil, &StepError{Reason: ReasonScriptFailed, Cause: err}
	}
	return ScriptOutput{}.Events(in, res.Value)
}

// ScriptOutput converts a transform result back into events.
type ScriptOutput struct{}

// Events maps null → [], object → [event], array of objects → events.
func (o ScriptOutput) Events(in engine.Event, v any) ([]engine.Event, error) {
	switch t := v.(type) {
	case nil:
		return []engine.Event{}, nil
	case map[string]any:
		return One(in.WithSnapshot(NumberNormalizer{}.Of(t).(map[string]any))), nil
	case []any:
		return o.list(in, t)
	}
	return nil, &StepError{Reason: ReasonScriptOutput, Cause: ErrScriptOutput}
}

func (ScriptOutput) list(in engine.Event, items []any) ([]engine.Event, error) {
	out := make([]engine.Event, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, &StepError{Reason: ReasonScriptOutput, Cause: ErrScriptOutput}
		}
		out = append(out, in.Clone().WithSnapshot(NumberNormalizer{}.Of(m).(map[string]any)))
	}
	return out, nil
}
