package operators

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// IsEmpty matches missing, null, "", [] and {}.
type IsEmpty struct{}

func (IsEmpty) Op() config.OpName           { return config.OpNameIsEmpty }
func (IsEmpty) Accepts() []filter.FieldType { return allTypes }
func (IsEmpty) Operand() filter.OperandKind { return filter.OperandNone }
func (IsEmpty) HandlesMissing() bool        { return true }
func (IsEmpty) Prepare(filter.Operand) (filter.Prepared, error) {
	return emptiness{}, nil
}

type emptiness struct{}

func (emptiness) Match(in filter.MatchInput) (bool, *filter.CoercionWarning) {
	if !in.Present || in.Left == nil {
		return true, nil
	}
	switch t := in.Left.(type) {
	case string:
		return t == "", nil
	case []any:
		return len(t) == 0, nil
	case map[string]any:
		return len(t) == 0, nil
	}
	return false, nil
}
