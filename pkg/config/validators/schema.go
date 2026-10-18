// Package validators holds the doc-only rule families of pkg/config: schema,
// delivery mode and dry-run compile. External-data rules live in internal/routes/validation.
package validators

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/eventrouter/router/pkg/config"
)

// SchemaValidator checks the document against the embedded router/v1 JSON Schema and
// halts later validators on failure.
type SchemaValidator struct {
	schema  *jsonschema.Schema
	printer *message.Printer
}

// NewSchemaValidator compiles the embedded schema once.
func NewSchemaValidator() (*SchemaValidator, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(config.SchemaCatalog{}.RouteV1()))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(string(config.SchemaIDRouteV1), doc); err != nil {
		return nil, err
	}
	sch, err := c.Compile(string(config.SchemaIDRouteV1))
	if err != nil {
		return nil, err
	}
	return &SchemaValidator{schema: sch, printer: message.NewPrinter(language.English)}, nil
}

func (v *SchemaValidator) Name() config.ValidatorName { return config.ValidatorSchema }

// Validate maps every leaf violation to a SchemaInvalid issue at its instance pointer.
func (v *SchemaValidator) Validate(_ context.Context, vc *config.ValidationContext) []config.Issue {
	err := v.schema.Validate(vc.Doc.Tree())
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		vc.Halt()
		return []config.Issue{config.NewError(config.IssueSchemaInvalid, config.PointerRoot, "%v", err)}
	}
	issues := violationSet{printer: v.printer}.collect(ve)
	vc.Halt()
	return issues
}

// violationSet flattens the cause tree into deduplicated, most-specific issues.
type violationSet struct {
	printer *message.Printer
}

func (s violationSet) collect(root *jsonschema.ValidationError) []config.Issue {
	leaves := s.leaves(root, nil)
	out := []config.Issue{}
	seen := map[string]bool{}
	for _, l := range leaves {
		ptr := config.PointerFromTokens(l.InstanceLocation)
		if s.shadowed(l, ptr, leaves) {
			continue
		}
		msg := l.ErrorKind.LocalizedString(s.printer)
		if seen[string(ptr)+msg] {
			continue
		}
		seen[string(ptr)+msg] = true
		out = append(out, config.NewError(config.IssueSchemaInvalid, ptr, "%s", msg))
	}
	return out
}

func (s violationSet) leaves(e *jsonschema.ValidationError, acc []*jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(e.Causes) == 0 {
		return append(acc, e)
	}
	for _, c := range e.Causes {
		acc = s.leaves(c, acc)
	}
	return acc
}

// shadowed drops type mismatches of oneOf alternatives when a deeper violation explains the node.
func (violationSet) shadowed(l *jsonschema.ValidationError, ptr config.Pointer, all []*jsonschema.ValidationError) bool {
	if _, isType := l.ErrorKind.(*kind.Type); !isType {
		return false
	}
	for _, o := range all {
		other := string(config.PointerFromTokens(o.InstanceLocation))
		if other != string(ptr) && strings.HasPrefix(other, string(ptr)+"/") {
			return true
		}
		if _, otherIsType := o.ErrorKind.(*kind.Type); other == string(ptr) && !otherIsType {
			return true
		}
	}
	return false
}

// MustSchemaValidator panics when the embedded schema does not compile (programming error).
func MustSchemaValidator() *SchemaValidator {
	v, err := NewSchemaValidator()
	if err != nil {
		panic(fmt.Sprintf("embedded route schema: %v", err))
	}
	return v
}
