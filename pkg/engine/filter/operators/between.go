package operators

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// Between matches from <= left <= to (inclusive).
type Between struct{ base }

func (Between) Op() config.OpName           { return config.OpNameBetween }
func (Between) Accepts() []filter.FieldType { return orderedTypes }
func (Between) Operand() filter.OperandKind { return filter.OperandRange }
func (Between) Prepare(o filter.Operand) (filter.Prepared, error) {
	if o.From == nil || o.To == nil {
		return nil, filter.NewOperandError(config.IssueOperandInvalid, config.TokenFrom, "between requires from and to")
	}
	return betweenMatch{from: o.From, to: o.To}, nil
}

type betweenMatch struct {
	from, to any
}

func (p betweenMatch) Match(in filter.MatchInput) (bool, *filter.CoercionWarning) {
	lo, w, ok := in.Coercer.Compare(in.Left, p.from)
	if !ok {
		return false, w
	}
	hi, w2, ok := in.Coercer.Compare(in.Left, p.to)
	if w == nil {
		w = w2
	}
	return ok && lo >= 0 && hi <= 0, w
}
