package recipes

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/filter"
	"github.com/eventrouter/router/pkg/engine/step"
)

// Converter converts a value to one target type.
type Converter interface {
	Target() config.ConvertTarget
	Convert(v any) (any, error)
}

// FailurePolicy decides what a failed conversion does.
type FailurePolicy interface {
	Mode() config.ConvertFailure
	Handle(ev *engine.Event, f step.FieldRef, cause error) error
}

// converters and failurePolicies are the registries of convert_type.
var converters = map[config.ConvertTarget]Converter{}
var failurePolicies = map[config.ConvertFailure]FailurePolicy{}

func init() {
	for _, c := range []Converter{toString{}, toNumber{}, toInteger{}, toBoolean{}} {
		converters[c.Target()] = c
	}
	for _, p := range []FailurePolicy{failError{}, failNull{}, failKeep{}} {
		failurePolicies[p.Mode()] = p
	}
}

// ConvertType converts a field to string, number, integer or boolean.
type ConvertType struct{}

func (ConvertType) Kind() config.StepKind { return config.StepKindConvertType }
func (ConvertType) Group() step.Group     { return step.GroupTypes }

func (ConvertType) Compile(c *step.StepCompileCtx, raw config.Step) (step.Step, bool) {
	target, ok := c.Path(raw.Field, config.TokenField)
	if raw.TargetType == nil || converters[*raw.TargetType] == nil {
		c.Fail(config.IssueSchemaInvalid, config.TokenTargetType, "convert_type requires a supported targetType")
		return nil, false
	}
	mode := config.ConvertFailureError
	if raw.OnFailure != nil {
		mode = *raw.OnFailure
	}
	return &convertType{Base: step.NewBase(c, raw), target: target, conv: converters[*raw.TargetType], onFailure: failurePolicies[mode]}, ok && failurePolicies[mode] != nil
}

type convertType struct {
	step.Base
	target    step.FieldRef
	conv      Converter
	onFailure FailurePolicy
}

func (s *convertType) Apply(_ *step.StepContext, in engine.Event) ([]engine.Event, error) {
	v, ok := s.target.Get(&in)
	if !ok {
		return step.One(in), s.Missing(s.target)
	}
	out, err := s.conv.Convert(v)
	if err != nil {
		return step.One(in), s.onFailure.Handle(&in, s.target, err)
	}
	if err := s.target.Write(&in, out); err != nil {
		return nil, err
	}
	return step.One(in), nil
}

type toString struct{}

func (toString) Target() config.ConvertTarget { return config.ConvertTargetString }
func (toString) Convert(v any) (any, error) {
	if b, ok := (codec.ScalarText{}).Of(v); ok && v != nil {
		return string(b), nil
	}
	b, err := json.Marshal(v)
	return string(b), err
}

type toNumber struct{}

func (toNumber) Target() config.ConvertTarget { return config.ConvertTargetNumber }
func (toNumber) Convert(v any) (any, error) {
	f, ok := (filter.Coercer{}).Number(v)
	if !ok {
		f, ok = (filter.Coercer{}).NumericString(v)
	}
	if !ok {
		return nil, fmt.Errorf("cannot convert %T to number", v)
	}
	if f == math.Trunc(f) && math.Abs(f) < math.MaxInt64 {
		return int64(f), nil
	}
	return f, nil
}

type toInteger struct{}

func (toInteger) Target() config.ConvertTarget { return config.ConvertTargetInteger }
func (toInteger) Convert(v any) (any, error) {
	n, err := toNumber{}.Convert(v)
	if err != nil {
		return nil, err
	}
	if i, ok := n.(int64); ok {
		return i, nil
	}
	return nil, fmt.Errorf("%v is not an integer", n)
}

type toBoolean struct{}

func (toBoolean) Target() config.ConvertTarget { return config.ConvertTargetBoolean }
func (toBoolean) Convert(v any) (any, error) {
	if s, ok := v.(string); ok {
		b, err := strconv.ParseBool(strings.TrimSpace(s))
		return b, err
	}
	if b, ok, _ := (filter.Coercer{}).Bool(v); ok {
		return b, nil
	}
	if f, ok := (filter.Coercer{}).Number(v); ok {
		return f != 0, nil
	}
	return nil, fmt.Errorf("cannot convert %T to boolean", v)
}

type failError struct{}

func (failError) Mode() config.ConvertFailure { return config.ConvertFailureError }
func (failError) Handle(_ *engine.Event, f step.FieldRef, cause error) error {
	return &step.StepError{Reason: step.ReasonConvertFailed, Field: f.String(), Cause: cause}
}

type failNull struct{}

func (failNull) Mode() config.ConvertFailure { return config.ConvertFailureNull }
func (failNull) Handle(ev *engine.Event, f step.FieldRef, _ error) error {
	return f.Write(ev, nil)
}

type failKeep struct{}

func (failKeep) Mode() config.ConvertFailure                      { return config.ConvertFailureKeep }
func (failKeep) Handle(*engine.Event, step.FieldRef, error) error { return nil }
