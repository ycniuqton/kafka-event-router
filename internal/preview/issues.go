package preview

import (
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/pkg/config"
)

// IssueMapper converts config issues to DTOs and problems.
type IssueMapper struct{}

// DTOs renders every issue.
func (IssueMapper) DTOs(is config.Issues) []IssueDTO {
	out := make([]IssueDTO, 0, len(is))
	for _, i := range is {
		out = append(out, IssueDTO{Path: string(i.Path), Severity: string(i.Severity), Code: string(i.Code), Message: i.Message})
	}
	return out
}

// InvalidConfigError is the 422 problem of a draft that does not compile; paths point into the
// request body (`/config/...`) and issues[] keeps the document-relative paths for the editor.
type InvalidConfigError struct {
	Issues config.Issues
}

func (e *InvalidConfigError) Error() string             { return string(DetailInvalidConfig) }
func (e *InvalidConfigError) ProblemKind() problem.Kind { return KindInvalidConfig }

// ProblemPath points at the first error inside the config member of the request.
func (e *InvalidConfigError) ProblemPath() string {
	errs := e.Issues.Errors()
	if len(errs) == 0 {
		return string(FieldConfig)
	}
	return string(FieldConfig) + string(errs[0].Path)
}

// FieldErrors lists every error issue with its request pointer.
func (e *InvalidConfigError) FieldErrors() []problem.FieldError {
	out := []problem.FieldError{}
	for _, i := range e.Issues.Errors() {
		out = append(out, problem.FieldError{Path: string(FieldConfig) + string(i.Path), Code: string(i.Code), Message: i.Message})
	}
	return out
}
