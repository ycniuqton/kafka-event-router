package operators

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// NotExists matches when the field is missing.
type NotExists struct{}

func (NotExists) Op() config.OpName           { return config.OpNameNotExists }
func (NotExists) Accepts() []filter.FieldType { return allTypes }
func (NotExists) Operand() filter.OperandKind { return filter.OperandNone }
func (NotExists) HandlesMissing() bool        { return true }
func (NotExists) Prepare(filter.Operand) (filter.Prepared, error) {
	return presence{want: false}, nil
}
