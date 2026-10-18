package operators

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// textMatch is a string-only operator defined by a predicate on (left, operand).
type textMatch struct {
	base
	op   config.OpName
	test func(s, operand string) bool
}

func (t textMatch) Op() config.OpName         { return t.op }
func (textMatch) Accepts() []filter.FieldType { return stringTypes }
func (textMatch) Operand() filter.OperandKind { return filter.OperandValue }
func (t textMatch) Prepare(o filter.Operand) (filter.Prepared, error) {
	s, ok := o.Value.(string)
	if !ok {
		return nil, filter.NewOperandError(config.IssueOperandInvalid, config.TokenValue, "%s requires a string value", t.op)
	}
	return textMatchPrepared{operand: s, test: t.test}, nil
}

type textMatchPrepared struct {
	operand string
	test    func(s, operand string) bool
}

func (p textMatchPrepared) Match(in filter.MatchInput) (bool, *filter.CoercionWarning) {
	s, ok := in.Left.(string)
	if !ok {
		return false, &filter.CoercionWarning{Code: filter.WarnIncomparable}
	}
	return p.test(s, p.operand), nil
}
