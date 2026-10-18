package operators

import (
	"strings"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// Contains matches a substring of a string or an element of an array.
type Contains struct{ base }

func (Contains) Op() config.OpName { return config.OpNameContains }
func (Contains) Accepts() []filter.FieldType {
	return []filter.FieldType{filter.FieldString, filter.FieldArray}
}
func (Contains) Operand() filter.OperandKind { return filter.OperandValue }
func (c Contains) Prepare(o filter.Operand) (filter.Prepared, error) {
	if err := c.requireValue(o); err != nil {
		return nil, err
	}
	return containsMatch{needle: o.Value}, nil
}

type containsMatch struct {
	needle any
}

func (p containsMatch) Match(in filter.MatchInput) (bool, *filter.CoercionWarning) {
	if arr, ok := in.Left.([]any); ok {
		return p.element(in.Coercer, arr), nil
	}
	s, sok := in.Left.(string)
	n, nok := p.needle.(string)
	if !sok || !nok {
		return false, &filter.CoercionWarning{Code: filter.WarnIncomparable}
	}
	return strings.Contains(s, n), nil
}

func (p containsMatch) element(cz filter.Coercer, arr []any) bool {
	for _, e := range arr {
		if ok, _ := cz.Equal(e, p.needle); ok {
			return true
		}
	}
	return false
}
