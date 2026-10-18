package operators

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// Exists matches when the field is present (null counts as present).
type Exists struct{}

func (Exists) Op() config.OpName           { return config.OpNameExists }
func (Exists) Accepts() []filter.FieldType { return allTypes }
func (Exists) Operand() filter.OperandKind { return filter.OperandNone }
func (Exists) HandlesMissing() bool        { return true }
func (Exists) Prepare(filter.Operand) (filter.Prepared, error) {
	return presence{want: true}, nil
}

// presence tests whether the path resolved.
type presence struct {
	want bool
}

func (p presence) Match(in filter.MatchInput) (bool, *filter.CoercionWarning) {
	return in.Present == p.want, nil
}
