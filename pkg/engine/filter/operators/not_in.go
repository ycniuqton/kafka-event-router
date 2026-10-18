package operators

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// NotIn matches when left equals none of the listed values.
type NotIn struct{ base }

func (NotIn) Op() config.OpName           { return config.OpNameNotIn }
func (NotIn) Accepts() []filter.FieldType { return scalarTypes }
func (NotIn) Operand() filter.OperandKind { return filter.OperandValues }
func (NotIn) Prepare(o filter.Operand) (filter.Prepared, error) {
	return membership{values: o.Values, negate: true}, nil
}
