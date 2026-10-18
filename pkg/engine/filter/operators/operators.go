// Package operators implements the 19 native cond operators, one file each.
package operators

import (
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// Default returns an OperatorManager with every MVP operator registered.
func Default() *filter.OperatorManager {
	return filter.NewOperatorManager(
		Eq{}, Ne{}, Gt(), Gte(), Lt(), Lte(), Between{}, In{}, NotIn{},
		Contains{}, StartsWith(), EndsWith(), Regex{}, Exists{}, NotExists{}, IsEmpty{},
		Before(), After(), WithinLast{},
	)
}

// base supplies the defaults shared by most operators.
type base struct{}

func (base) HandlesMissing() bool { return false }

// scalarTypes are the field types comparable by equality.
var scalarTypes = []filter.FieldType{filter.FieldString, filter.FieldNumber, filter.FieldBoolean, filter.FieldDate}

// orderedTypes are the field types with an ordering.
var orderedTypes = []filter.FieldType{filter.FieldNumber, filter.FieldString}

// dateTypes are the field types read as time.
var dateTypes = []filter.FieldType{filter.FieldDate}

// stringTypes are text-only field types.
var stringTypes = []filter.FieldType{filter.FieldString}

// allTypes lists every field type.
var allTypes = []filter.FieldType{filter.FieldString, filter.FieldNumber, filter.FieldBoolean, filter.FieldDate,
	filter.FieldObject, filter.FieldArray, filter.FieldNull}

// requireValue fails when the operand has no value.
func (base) requireValue(o filter.Operand) error {
	if o.Value == nil {
		return filter.NewOperandError(config.IssueOperandInvalid, config.TokenValue, "operator requires a value")
	}
	return nil
}
