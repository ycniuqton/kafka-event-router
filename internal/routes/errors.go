package routes

import (
	"fmt"

	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/pkg/config"
)

// ErrorPath names request fields that route errors point at.
type ErrorPath string

const (
	PathName     ErrorPath = "/name"
	PathDraft    ErrorPath = "/draft"
	PathTeamID   ErrorPath = "/teamId"
	PathVersion  ErrorPath = "/version"
	PathTag      ErrorPath = "/tag"
	PathIDs      ErrorPath = "/ids"
	PathAction   ErrorPath = "/action"
	PathReplicas ErrorPath = "/replicas"
	PathMode     ErrorPath = "/mode"
	PathTags     ErrorPath = "/tags"
)

var (
	// ErrRouteNotFound is returned for unknown route ids (404).
	ErrRouteNotFound = problem.NewError(problem.KindNotFound, "This route does not exist. It may have been archived or the link is wrong.")
	// ErrVersionNotFound is returned for unknown version numbers (404).
	ErrVersionNotFound = problem.NewError(problem.KindNotFound, "This version does not exist. Pick a version from the history.").At(string(PathVersion))
	// ErrDeploymentNotFound is returned for unknown deployment ids (404).
	ErrDeploymentNotFound = problem.NewError(problem.KindNotFound, "This deployment does not exist.")
	// ErrNameTaken is returned when another route of the team already uses the name (409).
	ErrNameTaken = problem.NewError(problem.KindConflict, "Another route of this team already uses this name. Pick a different name.").At(string(PathName))
	// ErrNothingToDeploy is returned when the draft equals the running version (409).
	ErrNothingToDeploy = problem.NewError(problem.KindConflict, "Nothing to deploy: the draft is identical to the running version. Change the draft first.")
	// ErrArchived is returned for state changes on archived routes (409).
	ErrArchived = problem.NewError(problem.KindConflict, "This route is archived. Archived routes cannot be changed or deployed.")
	// ErrNotDeployed is returned when pausing or resuming a route that was never deployed (409).
	ErrNotDeployed = problem.NewError(problem.KindConflict, "Draft routes are not running. Deploy the route first.")
	// ErrAlreadyActive is returned when rolling back to the live version (409).
	ErrAlreadyActive = problem.NewError(problem.KindConflict, "This version is already live. Pick an older version to roll back to.").At(string(PathVersion))
	// ErrTransitionNotAllowed is returned when a deployment is not in the expected status (409).
	ErrTransitionNotAllowed = problem.NewError(problem.KindConflict, "This deployment is no longer in a state that allows this change. Reload to see its status.")
	// ErrDraftNotJSON is returned when a draft is not a JSON object (422).
	ErrDraftNotJSON = problem.NewError(problem.KindValidation, "The draft must be a JSON object route document.").At(string(PathDraft))
	// ErrForbiddenTeam is returned when the caller has no write role on the owning team (403).
	ErrForbiddenTeam = problem.NewError(problem.KindForbidden, "You need the editor or admin role on the team that owns this route.")
	// ErrNoWritableTeam is returned on create when the caller can write to no team (403).
	ErrNoWritableTeam = problem.NewError(problem.KindForbidden, "You need the editor or admin role on a team to create routes.").At(string(PathTeamID))
	// ErrBadVersionRef is returned for diff references that are neither a number, draft nor active (400).
	ErrBadVersionRef = problem.NewError(problem.KindMalformedBody, "Version references are a number, \"active\" or \"draft\".")
	// ErrUnknownBulkAction is returned for unsupported bulk actions (400).
	ErrUnknownBulkAction = problem.NewError(problem.KindMalformedBody, "Unknown bulk action. Use pause, resume, add_tag or export.").At(string(PathAction))
	// ErrNoDraft is returned when validating a route that has no draft (409).
	ErrNoDraft = problem.NewError(problem.KindConflict, "This route has no draft to validate. Send a draft or save one first.")
	// ErrBadMode is returned for unknown validation modes (400).
	ErrBadMode = problem.NewError(problem.KindMalformedBody, "Unknown validation mode. Use draft, deploy, rollback or import.").At(string(PathMode))
	// ErrTooManyIDs is returned for bulk requests above MaxBulkIDs (422).
	ErrTooManyIDs = problem.NewError(problem.KindValidation, fmt.Sprintf("Select at most %d routes per bulk action.", MaxBulkIDs)).At(string(PathIDs))
)

// InvalidConfigError carries validation issues that block an operation (422 with `issues`).
type InvalidConfigError struct {
	Issues config.Issues
}

// NewInvalidConfigError wraps blocking issues.
func NewInvalidConfigError(issues config.Issues) *InvalidConfigError {
	return &InvalidConfigError{Issues: issues}
}

func (e *InvalidConfigError) Error() string {
	return fmt.Sprintf("The route has %d blocking issue(s). Fix them and retry.", len(e.Issues.Errors()))
}

// ProblemKind implements problem.Classified.
func (e *InvalidConfigError) ProblemKind() problem.Kind { return problem.KindValidation }

// ProblemPath points at the first blocking issue.
func (e *InvalidConfigError) ProblemPath() string {
	errs := e.Issues.Errors()
	if len(errs) == 0 {
		return ""
	}
	return string(errs[0].Path)
}

// FieldErrors implements problem.Itemised with the blocking issues.
func (e *InvalidConfigError) FieldErrors() []problem.FieldError {
	out := []problem.FieldError{}
	for _, is := range e.Issues.Errors() {
		out = append(out, problem.FieldError{Path: string(is.Path), Code: string(is.Code), Message: is.Message})
	}
	return out
}
