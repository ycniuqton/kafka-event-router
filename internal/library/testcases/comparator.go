package testcases

import (
	"encoding/json"
	"fmt"

	"github.com/wI2L/jsondiff"

	"github.com/eventrouter/router/pkg/engine/trace"
)

// ComparatorKind names an OutcomeComparator implementation (selected by configuration).
type ComparatorKind string

const ComparatorExactJSON ComparatorKind = "exact_json"

// Observation is an outcome with its output (null unless the outcome carries one).
type Observation struct {
	Outcome Outcome
	Output  json.RawMessage
}

// Comparison is the verdict of one comparison; Diff turns the expected output into the actual one.
type Comparison struct {
	Equal   bool
	Diff    []trace.PatchOp
	Message string
}

// OutcomeComparator compares an expected observation with the actual one.
type OutcomeComparator interface {
	Kind() ComparatorKind
	Compare(expected, actual Observation) Comparison
}

// ComparatorManager holds the registered comparators and the active one.
type ComparatorManager struct {
	comparators map[ComparatorKind]OutcomeComparator
	active      OutcomeComparator
}

// NewComparatorManager registers comparators and selects kind (ErrUnknownComparator if absent).
func NewComparatorManager(kind ComparatorKind, comparators ...OutcomeComparator) (*ComparatorManager, error) {
	m := &ComparatorManager{comparators: map[ComparatorKind]OutcomeComparator{}}
	for _, c := range comparators {
		m.comparators[c.Kind()] = c
	}
	active, ok := m.comparators[kind]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownComparator, kind)
	}
	m.active = active
	return m, nil
}

// DefaultComparatorManager selects the exact JSON comparator.
func DefaultComparatorManager() *ComparatorManager {
	m, _ := NewComparatorManager(ComparatorExactJSON, ExactJSONComparator{})
	return m
}

// Compare delegates to the active comparator.
func (m *ComparatorManager) Compare(expected, actual Observation) Comparison {
	return m.active.Compare(expected, actual)
}

// ExactJSONComparator requires the same outcome and, for passed events, JSON-equal outputs:
// key order is ignored, numbers compare by value, null and a missing member differ.
type ExactJSONComparator struct{}

// Kind implements OutcomeComparator.
func (ExactJSONComparator) Kind() ComparatorKind { return ComparatorExactJSON }

// Compare implements OutcomeComparator.
func (c ExactJSONComparator) Compare(expected, actual Observation) Comparison {
	if expected.Outcome != actual.Outcome {
		return Comparison{Diff: []trace.PatchOp{}, Message: fmt.Sprintf(string(MsgOutcomeMismatch), expected.Outcome, actual.Outcome)}
	}
	if !expected.Outcome.CarriesOutput() {
		return Comparison{Equal: true, Diff: []trace.PatchOp{}}
	}
	diff := c.diff(JSONValue{}.Of(expected.Output), JSONValue{}.Of(actual.Output))
	if len(diff) == 0 {
		return Comparison{Equal: true, Diff: diff}
	}
	return Comparison{Diff: diff, Message: fmt.Sprintf(string(MsgOutputMismatch), len(diff))}
}

// diff returns the RFC 6902 patch from want to got (a whole-document replace if diffing fails).
func (ExactJSONComparator) diff(want, got any) []trace.PatchOp {
	patch, err := jsondiff.Compare(want, got)
	if err != nil {
		return []trace.PatchOp{{Op: string(trace.PatchReplace), Path: "", Value: got, HasValue: true}}
	}
	out := make([]trace.PatchOp, 0, len(patch))
	for _, op := range patch {
		out = append(out, trace.PatchOp{Op: op.Type, From: op.From, Path: op.Path, Value: op.Value, HasValue: op.Type != string(jsondiff.OperationRemove)})
	}
	return out
}

// JSONValue decodes raw JSON into generic values (numbers as float64, so 1 and 1.0 are equal).
type JSONValue struct{}

// Of decodes raw; empty or invalid input is null.
func (JSONValue) Of(raw json.RawMessage) any {
	var v any
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}
