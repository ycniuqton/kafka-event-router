package operators

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// Ne is inequality; a missing field is false like every value operator.
type Ne struct{ base }

func (Ne) Op() config.OpName           { return config.OpNameNe }
func (Ne) Accepts() []filter.FieldType { return scalarTypes }
func (Ne) Operand() filter.OperandKind { return filter.OperandValue }
func (Ne) Prepare(o filter.Operand) (filter.Prepared, error) {
	return equality{right: o.Value, negate: true}, nil
}
