package validation

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// operatorTypes lists the field types each typed operator accepts; operators not listed accept any type.
var operatorTypes = map[config.OpName][]FieldType{
	config.OpNameGt:         {FieldNumber, FieldTimestamp},
	config.OpNameGte:        {FieldNumber, FieldTimestamp},
	config.OpNameLt:         {FieldNumber, FieldTimestamp},
	config.OpNameLte:        {FieldNumber, FieldTimestamp},
	config.OpNameBetween:    {FieldNumber, FieldTimestamp},
	config.OpNameContains:   {FieldString, FieldArray},
	config.OpNameStartsWith: {FieldString},
	config.OpNameEndsWith:   {FieldString},
	config.OpNameRegex:      {FieldString},
	config.OpNameBefore:     {FieldTimestamp, FieldString, FieldNumber},
	config.OpNameAfter:      {FieldTimestamp, FieldString, FieldNumber},
	config.OpNameWithinLast: {FieldTimestamp, FieldString, FieldNumber},
	config.OpNameIsEmpty:    {FieldString, FieldArray, FieldObject, FieldNull},
}

// FieldTypeValidator warns about filter fields unseen in samples and operator/type mismatches.
type FieldTypeValidator struct {
	Schemas SampleSchemaProvider
}

func (FieldTypeValidator) Name() ValidatorName     { return ValidatorFieldType }
func (FieldTypeValidator) Modes() []ValidationMode { return []ValidationMode{ModeDraft, ModeDeploy} }

// Validate checks each branch filter against the latest sample schema; no samples → no issues.
func (v FieldTypeValidator) Validate(ctx context.Context, in ValidationInput) ([]config.Issue, error) {
	if in.Config == nil || !(BranchFilters{}).Any(in.Config) {
		return nil, nil
	}
	req := SchemaRequest{TeamID: in.TeamID, ConnectionID: in.Config.Source.ConnectionID, Topics: in.Config.Source.Topics}
	schema, ok, err := v.Schemas.LatestSchema(ctx, req)
	if err != nil || !ok {
		return nil, err
	}
	w := filterWalker{schema: schema}
	for i, b := range in.Config.Branches {
		if b.Filter != nil {
			w.walk(b.Filter, config.BranchPointer(i).Token(config.TokenFilter))
		}
	}
	return w.issues, nil
}

// BranchFilters answers questions about the filters of a config.
type BranchFilters struct{}

// Any reports whether at least one branch has a filter.
func (BranchFilters) Any(cfg *config.RouteConfig) bool {
	for _, b := range cfg.Branches {
		if b.Filter != nil {
			return true
		}
	}
	return false
}

// filterWalker visits filter nodes and collects field warnings.
type filterWalker struct {
	schema FieldSchema
	issues []config.Issue
}

func (w *filterWalker) walk(n *config.FilterNode, at config.Pointer) {
	switch n.Type {
	case config.NodeTypeAnd, config.NodeTypeOr:
		for i := range n.Children {
			w.walk(&n.Children[i], at.Token(config.TokenChildren).Index(i))
		}
	case config.NodeTypeNot:
		if n.Child != nil {
			w.walk(n.Child, at.Token(config.TokenChild))
		}
	case config.NodeTypeCond:
		w.cond(n, at)
	}
}

func (w *filterWalker) cond(n *config.FilterNode, at config.Pointer) {
	if n.Field == nil || n.Op == nil {
		return
	}
	typ, known := w.schema[*n.Field]
	if !known {
		if *n.Op != config.OpNameExists && *n.Op != config.OpNameNotExists {
			w.issues = append(w.issues, config.NewWarning(config.IssueFieldUnknown, at.Token(config.TokenField),
				"Field %q was not seen in the latest samples. Check the spelling.", *n.Field))
		}
		return
	}
	allowed, typed := operatorTypes[*n.Op]
	if typed && !FieldTypes(allowed).Contains(typ) {
		w.issues = append(w.issues, config.NewWarning(config.IssueOperatorTypeMismatch, at.Token(config.TokenOp),
			"Operator %q does not fit field %q, which holds %s values in the samples.", *n.Op, *n.Field, typ))
	}
}

// FieldTypes is a set of field types.
type FieldTypes []FieldType

// Contains reports whether t is listed.
func (ts FieldTypes) Contains(t FieldType) bool {
	for _, x := range ts {
		if x == t {
			return true
		}
	}
	return false
}
