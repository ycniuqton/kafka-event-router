package operators

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// Eq is equality with the fixed coercion table.
type Eq struct{ base }

func (Eq) Op() config.OpName           { return config.OpNameEq }
func (Eq) Accepts() []filter.FieldType { return scalarTypes }
func (Eq) Operand() filter.OperandKind { return filter.OperandValue }
func (e Eq) Prepare(o filter.Operand) (filter.Prepared, error) {
	return equality{right: o.Value, negate: false}, nil
}

// equality compares the left value with a constant.
type equality struct {
	right  any
	negate bool
}

func (p equality) Match(in filter.MatchInput) (bool, *filter.CoercionWarning) {
	ok, w := in.Coercer.Equal(in.Left, p.right)
	return ok != p.negate, w
}
