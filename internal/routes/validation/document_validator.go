package validation

import (
	"context"

	"github.com/eventrouter/router/pkg/config"
)

// DocumentChecker is pkg/config's doc-only ValidatorManager (schema, delivery mode, compile).
type DocumentChecker interface {
	ValidateDocument(ctx context.Context, stage config.ValidationStage, teamID string, doc *config.Document) (config.Issues, error)
}

// stageOfMode maps routes modes to pkg/config profile stages.
var stageOfMode = map[ValidationMode]config.ValidationStage{
	ModeDraft:    config.StageDraft,
	ModeImport:   config.StageDraft,
	ModeDeploy:   config.StageDeploy,
	ModeRollback: config.StageDeploy,
}

// DocumentValidator runs the doc-only profile of the mode's stage.
type DocumentValidator struct {
	Checker DocumentChecker
}

func (DocumentValidator) Name() ValidatorName     { return ValidatorDocument }
func (DocumentValidator) Modes() []ValidationMode { return AllModes }

// Validate reports a syntax issue for unparseable documents, else the doc-only issues.
func (v DocumentValidator) Validate(ctx context.Context, in ValidationInput) ([]config.Issue, error) {
	if in.Doc == nil {
		return []config.Issue{config.NewError(config.IssueSchemaInvalid, config.PointerRoot, "The route document is not valid JSON.")}, nil
	}
	return v.Checker.ValidateDocument(ctx, stageOfMode[in.Mode], in.TeamID, in.Doc)
}
