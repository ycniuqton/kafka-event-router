package operators

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// In matches when left equals any listed value.
type In struct{ base }

func (In) Op() config.OpName           { return config.OpNameIn }
func (In) Accepts() []filter.FieldType { return scalarTypes }
func (In) Operand() filter.OperandKind { return filter.OperandValues }
func (In) Prepare(o filter.Operand) (filter.Prepared, error) {
	return membership{values: o.Values, negate: false}, nil
}

// membership tests left against a list of constants.
type membership struct {
	values []any
	negate bool
}

func (p membership) Match(in filter.MatchInput) (bool, *filter.CoercionWarning) {
	var warning *filter.CoercionWarning
	for _, v := range p.values {
		ok, w := in.Coercer.Equal(in.Left, v)
		if ok {
			return !p.negate, w
		}
		if warning == nil && w != nil && w.Code != filter.WarnIncomparable {
			warning = w
		}
	}
	return p.negate, warning
}
