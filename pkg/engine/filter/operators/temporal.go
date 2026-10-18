package operators

import (
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// temporal compares the left time with a constant time.
type temporal struct {
	base
	op   config.OpName
	test func(left, operand time.Time) bool
}

func (t temporal) Op() config.OpName         { return t.op }
func (temporal) Accepts() []filter.FieldType { return dateTypes }
func (temporal) Operand() filter.OperandKind { return filter.OperandValue }
func (t temporal) Prepare(o filter.Operand) (filter.Prepared, error) {
	at, ok := filter.Coercer{}.Time(o.Value)
	if !ok {
		return nil, filter.NewOperandError(config.IssueOperandInvalid, config.TokenValue, "%s requires an RFC 3339 time or epoch ms", t.op)
	}
	return temporalMatch{at: at, test: t.test}, nil
}

type temporalMatch struct {
	at   time.Time
	test func(left, operand time.Time) bool
}

func (p temporalMatch) Match(in filter.MatchInput) (bool, *filter.CoercionWarning) {
	left, ok := in.Coercer.Time(in.Left)
	if !ok {
		return false, &filter.CoercionWarning{Code: filter.WarnNotADate}
	}
	return p.test(left, p.at), nil
}
