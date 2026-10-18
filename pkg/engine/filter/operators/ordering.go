package operators

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// ordering is a comparison operator defined by which compare results match.
type ordering struct {
	base
	op      config.OpName
	accepts func(cmp int) bool
}

func (o ordering) Op() config.OpName         { return o.op }
func (ordering) Accepts() []filter.FieldType { return orderedTypes }
func (ordering) Operand() filter.OperandKind { return filter.OperandValue }
func (o ordering) Prepare(op filter.Operand) (filter.Prepared, error) {
	if err := o.requireValue(op); err != nil {
		return nil, err
	}
	return orderingMatch{right: op.Value, accepts: o.accepts}, nil
}

type orderingMatch struct {
	right   any
	accepts func(cmp int) bool
}

func (p orderingMatch) Match(in filter.MatchInput) (bool, *filter.CoercionWarning) {
	cmp, w, ok := in.Coercer.Compare(in.Left, p.right)
	if !ok {
		return false, w
	}
	return p.accepts(cmp), w
}
