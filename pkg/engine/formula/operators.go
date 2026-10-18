package formula

import (
	"math"

	"github.com/eventrouter/router/pkg/engine/filter"
)

// Precedence orders binary operators; higher binds tighter.
type Precedence int

const (
	PrecLowest     Precedence = 0
	PrecOr         Precedence = 1
	PrecAnd        Precedence = 2
	PrecEquality   Precedence = 3
	PrecComparison Precedence = 4
	PrecAdditive   Precedence = 5
	PrecMultiply   Precedence = 6
)

// BinaryOperator is one infix operator; operands are expressions so logic can short-circuit.
type BinaryOperator interface {
	Symbol() Symbol
	Precedence() Precedence
	Apply(ec *EvalContext, left, right Expr) (any, error)
}

// UnaryOperator is one prefix operator.
type UnaryOperator interface {
	Symbol() Symbol
	Apply(v any) (any, error)
}

// OperatorSet is the registry of formula operators selected by symbol.
type OperatorSet struct {
	binary map[Symbol]BinaryOperator
	unary  map[Symbol]UnaryOperator
}

// NewOperatorSet registers the given operators.
func NewOperatorSet(bs []BinaryOperator, us []UnaryOperator) *OperatorSet {
	s := &OperatorSet{binary: map[Symbol]BinaryOperator{}, unary: map[Symbol]UnaryOperator{}}
	for _, b := range bs {
		s.binary[b.Symbol()] = b
	}
	for _, u := range us {
		s.unary[u.Symbol()] = u
	}
	return s
}

// DefaultOperatorSet returns arithmetic, comparison and logic operators.
func DefaultOperatorSet() *OperatorSet {
	return NewOperatorSet(
		[]BinaryOperator{
			arithmetic{sym: SymPlus, prec: PrecAdditive, calc: func(a, b float64) (float64, error) { return a + b, nil }},
			arithmetic{sym: SymMinus, prec: PrecAdditive, calc: func(a, b float64) (float64, error) { return a - b, nil }},
			arithmetic{sym: SymStar, prec: PrecMultiply, calc: func(a, b float64) (float64, error) { return a * b, nil }},
			arithmetic{sym: SymSlash, prec: PrecMultiply, calc: Division{}.Quotient},
			arithmetic{sym: SymPercent, prec: PrecMultiply, calc: Division{}.Remainder},
			equality{sym: SymEq, want: true}, equality{sym: SymNe, want: false},
			comparison{sym: SymLt, accept: func(c int) bool { return c < 0 }},
			comparison{sym: SymLte, accept: func(c int) bool { return c <= 0 }},
			comparison{sym: SymGt, accept: func(c int) bool { return c > 0 }},
			comparison{sym: SymGte, accept: func(c int) bool { return c >= 0 }},
			logic{sym: SymAnd, prec: PrecAnd, shortOn: false}, logic{sym: SymOr, prec: PrecOr, shortOn: true},
		},
		[]UnaryOperator{negate{}, not{}},
	)
}

// Symbols lists every operator spelling (catalogue), binary first, in a stable order.
func (s *OperatorSet) Symbols() []Symbol {
	out := []Symbol{}
	for _, sym := range append(append([]Symbol{}, twoCharSymbols...), oneCharSymbols...) {
		_, b := s.binary[sym]
		_, u := s.unary[sym]
		if b || u {
			out = append(out, sym)
		}
	}
	return out
}

// Binary returns the infix operator of sym.
func (s *OperatorSet) Binary(sym Symbol) (BinaryOperator, bool) {
	b, ok := s.binary[sym]
	return b, ok
}

// Unary returns the prefix operator of sym.
func (s *OperatorSet) Unary(sym Symbol) (UnaryOperator, bool) {
	u, ok := s.unary[sym]
	return u, ok
}

// Division implements `/` and `%` with a zero check.
type Division struct{}

// Quotient divides a by b.
func (Division) Quotient(a, b float64) (float64, error) {
	if b == 0 {
		return 0, ErrDivisionByZero
	}
	return a / b, nil
}

// Remainder returns the floating-point remainder of a / b.
func (Division) Remainder(a, b float64) (float64, error) {
	if b == 0 {
		return 0, ErrDivisionByZero
	}
	return math.Mod(a, b), nil
}

// OperandPair evaluates both operands of a binary operator.
type OperandPair struct{}

// Eval returns the left and right values.
func (OperandPair) Eval(ec *EvalContext, left, right Expr) (any, any, error) {
	l, err := left.Eval(ec)
	if err != nil {
		return nil, nil, err
	}
	r, err := right.Eval(ec)
	return l, r, err
}

// arithmetic applies a numeric function; integer operands keep integer results when whole.
type arithmetic struct {
	sym  Symbol
	prec Precedence
	calc func(a, b float64) (float64, error)
}

func (o arithmetic) Symbol() Symbol         { return o.sym }
func (o arithmetic) Precedence() Precedence { return o.prec }

func (o arithmetic) Apply(ec *EvalContext, left, right Expr) (any, error) {
	l, r, err := OperandPair{}.Eval(ec, left, right)
	if err != nil {
		return nil, err
	}
	a, err := Numbers{}.Of(l)
	if err != nil {
		return nil, err
	}
	b, err := Numbers{}.Of(r)
	if err != nil {
		return nil, err
	}
	res, err := o.calc(a, b)
	if err != nil {
		return nil, err
	}
	integral := Numbers{}.IsInteger(l) && Numbers{}.IsInteger(r) && o.sym != SymSlash
	return Numbers{}.Result(res, integral), nil
}

// equality compares with the engine coercion table.
type equality struct {
	sym  Symbol
	want bool
}

func (o equality) Symbol() Symbol       { return o.sym }
func (equality) Precedence() Precedence { return PrecEquality }

func (o equality) Apply(ec *EvalContext, left, right Expr) (any, error) {
	l, r, err := OperandPair{}.Eval(ec, left, right)
	if err != nil {
		return nil, err
	}
	if l == nil || r == nil {
		return (l == nil && r == nil) == o.want, nil
	}
	eq, _ := (filter.Coercer{}).Equal(l, r)
	return eq == o.want, nil
}

// comparison orders numbers or strings; incomparable operands are false.
type comparison struct {
	sym    Symbol
	accept func(c int) bool
}

func (o comparison) Symbol() Symbol       { return o.sym }
func (comparison) Precedence() Precedence { return PrecComparison }

func (o comparison) Apply(ec *EvalContext, left, right Expr) (any, error) {
	l, r, err := OperandPair{}.Eval(ec, left, right)
	if err != nil {
		return nil, err
	}
	c, _, ok := (filter.Coercer{}).Compare(l, r)
	return ok && o.accept(c), nil
}

// logic is && and || with short-circuit; the result is a boolean.
type logic struct {
	sym     Symbol
	prec    Precedence
	shortOn bool
}

func (o logic) Symbol() Symbol         { return o.sym }
func (o logic) Precedence() Precedence { return o.prec }

func (o logic) Apply(ec *EvalContext, left, right Expr) (any, error) {
	l, err := left.Eval(ec)
	if err != nil {
		return nil, err
	}
	if (Truth{}).Of(l) == o.shortOn {
		return o.shortOn, nil
	}
	r, err := right.Eval(ec)
	if err != nil {
		return nil, err
	}
	return (Truth{}).Of(r), nil
}

// negate is unary minus.
type negate struct{}

func (negate) Symbol() Symbol { return SymMinus }

func (negate) Apply(v any) (any, error) {
	f, err := Numbers{}.Of(v)
	if err != nil {
		return nil, err
	}
	return Numbers{}.Result(-f, Numbers{}.IsInteger(v)), nil
}

// not is logical negation by truthiness.
type not struct{}

func (not) Symbol() Symbol { return SymNot }

func (not) Apply(v any) (any, error) { return !(Truth{}).Of(v), nil }
