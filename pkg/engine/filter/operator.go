package filter

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/eventrouter/router/pkg/config"
)

// FieldType is the inferred type of a sample field (operator applicability, catalogue).
type FieldType string

const (
	FieldString  FieldType = "string"
	FieldNumber  FieldType = "number"
	FieldBoolean FieldType = "boolean"
	FieldDate    FieldType = "date"
	FieldObject  FieldType = "object"
	FieldArray   FieldType = "array"
	FieldNull    FieldType = "null"
)

// OperandKind names which operand members an operator reads.
type OperandKind string

const (
	OperandNone     OperandKind = "none"
	OperandValue    OperandKind = "value"
	OperandValues   OperandKind = "values"
	OperandRange    OperandKind = "range"
	OperandDuration OperandKind = "duration"
)

// Operand is the right-hand side of a cond node.
type Operand struct {
	Value  any
	Values []any
	From   any
	To     any
	Amount *float64
	Unit   *config.DurationUnit
}

// MatchInput is the left value of one evaluation plus the frozen clock.
type MatchInput struct {
	Left    any
	Present bool
	Now     time.Time
	Coercer Coercer
}

// Prepared is an operator bound to a parsed operand (compiled once per version).
type Prepared interface {
	Match(in MatchInput) (bool, *CoercionWarning)
}

// Operator is one cond operator.
type Operator interface {
	Op() config.OpName
	Accepts() []FieldType
	Operand() OperandKind
	HandlesMissing() bool
	Prepare(o Operand) (Prepared, error)
}

// OperandError is a Prepare failure with the issue code and operand member it concerns.
type OperandError struct {
	Code   config.IssueCode
	Member config.PointerToken
	Msg    string
}

func (e *OperandError) Error() string { return e.Msg }

// NewOperandError builds an OperandError.
func NewOperandError(code config.IssueCode, member config.PointerToken, format string, args ...any) *OperandError {
	return &OperandError{Code: code, Member: member, Msg: fmt.Sprintf(format, args...)}
}

// OperandErrorInfo maps Prepare errors to issue code and pointer.
type OperandErrorInfo struct{}

// Of returns the code and pointer for err under the node pointer.
func (OperandErrorInfo) Of(err error, node config.Pointer) (config.IssueCode, config.Pointer) {
	var oe *OperandError
	if errors.As(err, &oe) {
		return oe.Code, node.Token(oe.Member)
	}
	return config.IssueOperandInvalid, node
}

// OperatorManager selects operators by name; business logic uses only this.
type OperatorManager struct {
	ops map[config.OpName]Operator
}

// NewOperatorManager registers the given operators.
func NewOperatorManager(ops ...Operator) *OperatorManager {
	m := &OperatorManager{ops: map[config.OpName]Operator{}}
	for _, o := range ops {
		m.ops[o.Op()] = o
	}
	return m
}

// Get returns the operator for op.
func (m *OperatorManager) Get(op config.OpName) (Operator, error) {
	o, ok := m.ops[op]
	if !ok {
		return nil, fmt.Errorf("unknown operator %q", op)
	}
	return o, nil
}

// All returns the operators sorted by name (catalogue).
func (m *OperatorManager) All() []Operator {
	out := make([]Operator, 0, len(m.ops))
	for _, o := range m.ops {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Op() < out[j].Op() })
	return out
}
