package infer

import (
	"encoding/json"
)

// MappingInferrer is the transform-by-example service: input event + desired output → proposed steps.
type MappingInferrer struct {
	matchers *MatcherManager
	emitter  StepEmitter
	limits   Limits
}

// NewMappingInferrer builds the inferrer over a matcher chain.
func NewMappingInferrer(matchers *MatcherManager, limits Limits) *MappingInferrer {
	return &MappingInferrer{matchers: matchers, limits: limits}
}

// Infer flattens both trees, matches each target leaf, and emits steps with per-target confidence.
func (m *MappingInferrer) Infer(req Request) (Result, error) {
	input, err := m.inputTree(req.Input)
	if err != nil {
		return Result{}, err
	}
	desired, ok := LeafFlattener{Limits: m.limits}.Flatten(req.DesiredOutput)
	if !ok {
		return Result{}, ErrDesiredNotObject
	}
	if len(desired.Leaves) >= m.limits.MaxLeaves || len(input.Leaves) >= m.limits.MaxLeaves {
		return Result{}, ErrTooLarge
	}
	pool := NewSourcePool(input.Leaves, desired.Leaves)
	matches := make([]Match, 0, len(desired.Leaves))
	for _, target := range desired.Leaves {
		matches = append(matches, m.match(target, pool))
	}
	emission := m.emitter.Emit(matches, input, pool)
	steps, err := StepJSON{}.Of(emission.Steps)
	if err != nil {
		return Result{}, err
	}
	return Result{Steps: steps, Mappings: MappingView{}.Of(matches), Unmapped: emission.Unmapped}, nil
}

// match finds the source of one target; a same-path source with another value becomes a constant.
func (m *MappingInferrer) match(target Leaf, pool *SourcePool) Match {
	found, ok := m.matchers.Match(target, pool)
	if !ok {
		return Match{Target: target, Reason: ReasonConstant, Confidence: ConfidenceConstant}
	}
	if found.Source.Path() != target.Path() || (Values{}).Key(found.Source.Value) == (Values{}).Key(target.Value) {
		return found
	}
	pool.Release(pool.Index(found.Source))
	return Match{Target: target, Reason: ReasonConstant, Confidence: ConfidenceConstant}
}

// inputTree decodes the event and flattens its value object.
func (m *MappingInferrer) inputTree(raw json.RawMessage) (Tree, error) {
	var event struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return Tree{}, ErrInputNotEvent
	}
	tree, ok := LeafFlattener{Limits: m.limits}.Flatten(event.Value)
	if !ok {
		return Tree{}, ErrInputNotEvent
	}
	return tree, nil
}

// MappingView renders matches for the response.
type MappingView struct{}

// Of returns one mapping per target leaf.
func (MappingView) Of(matches []Match) []Mapping {
	out := make([]Mapping, 0, len(matches))
	for _, mt := range matches {
		mp := Mapping{Target: mt.Target.Path(), Reason: mt.Reason, Confidence: mt.Confidence}
		if mt.Source != nil {
			mp.Source = mt.Source.Path()
		}
		out = append(out, mp)
	}
	return out
}
