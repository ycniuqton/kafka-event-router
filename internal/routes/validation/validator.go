// Package validation runs the external-data rules of a route (connections, topics, field types,
// names, later test cases and library refs) next to pkg/config's doc-only rules. Business code
// talks only to ValidationManager; each rule is a ContextValidator registered in a ValidatorSet.
package validation

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// ValidationMode tells which operation a validation run guards.
type ValidationMode string

const (
	ModeDraft    ValidationMode = "draft"
	ModeDeploy   ValidationMode = "deploy"
	ModeRollback ValidationMode = "rollback"
	ModeImport   ValidationMode = "import"
)

// AllModes lists every mode.
var AllModes = []ValidationMode{ModeDraft, ModeDeploy, ModeRollback, ModeImport}

// IsKnown reports whether m is a supported mode.
func (m ValidationMode) IsKnown() bool {
	for _, k := range AllModes {
		if k == m {
			return true
		}
	}
	return false
}

// ValidatorName identifies a context validator (used in "check skipped" warnings).
type ValidatorName string

const (
	ValidatorDocument         ValidatorName = "document"
	ValidatorConnectionAccess ValidatorName = "connection_access"
	ValidatorDestinationTopic ValidatorName = "destination_topic"
	ValidatorUniqueName       ValidatorName = "unique_name"
	ValidatorFieldType        ValidatorName = "field_type"
	ValidatorTestCase         ValidatorName = "test_case"
	ValidatorLibraryRef       ValidatorName = "library_ref"
	ValidatorSchemaRegistry   ValidatorName = "schema_registry"
	ValidatorSourceStart      ValidatorName = "source_start"
	ValidatorExactlyOnce      ValidatorName = "exactly_once"
)

// IssueCheckSkipped is the warning emitted when a validator fails or times out.
const IssueCheckSkipped config.IssueCode = "check_skipped"

// IssueNameMismatch warns that the document name differs from the route name.
const IssueNameMismatch config.IssueCode = "name_mismatch"

// IssueNameTaken reports a document name already used by another route of the team.
const IssueNameTaken config.IssueCode = "name_taken"

// ValidationInput is what every validator sees; Config is nil when the document cannot be decoded.
type ValidationInput struct {
	Mode      ValidationMode
	TeamID    string
	RouteID   string
	RouteName string
	Raw       []byte
	Doc       *config.Document
	Config    *config.RouteConfig
}

// ContextValidator checks one rule family; an error means "could not check" (→ warning).
type ContextValidator interface {
	Name() ValidatorName
	Modes() []ValidationMode
	Validate(ctx context.Context, in ValidationInput) ([]config.Issue, error)
}

// ValidatorSet is the registered validators, filtered per mode.
type ValidatorSet struct {
	validators []ContextValidator
}

// NewValidatorSet registers validators in order.
func NewValidatorSet(vs ...ContextValidator) *ValidatorSet {
	return &ValidatorSet{validators: vs}
}

// Add registers another validator (e.g. a later phase's rule).
func (s *ValidatorSet) Add(v ContextValidator) {
	s.validators = append(s.validators, v)
}

// For returns the validators that run in mode.
func (s *ValidatorSet) For(mode ValidationMode) []ContextValidator {
	out := []ContextValidator{}
	for _, v := range s.validators {
		if ModeList(v.Modes()).Contains(mode) {
			out = append(out, v)
		}
	}
	return out
}

// ModeList is a set of modes.
type ModeList []ValidationMode

// Contains reports whether m is listed.
func (l ModeList) Contains(m ValidationMode) bool {
	for _, x := range l {
		if x == m {
			return true
		}
	}
	return false
}
