package config

import (
	"fmt"
	"sort"
)

// IssueCode is the stable machine code of a validation finding, shared by
// pkg/config (doc-only rules) and internal/routes/validation (external-data rules).
type IssueCode string

const (
	IssueSchemaInvalid           IssueCode = "schema_invalid"
	IssueConnectionNotFound      IssueCode = "connection_not_found"
	IssueConnectionForbidden     IssueCode = "connection_forbidden"
	IssueFieldUnknown            IssueCode = "field_unknown"
	IssueOperatorTypeMismatch    IssueCode = "operator_type_mismatch"
	IssuePathInvalid             IssueCode = "path_invalid"
	IssueRegexInvalid            IssueCode = "regex_invalid"
	IssueOperandInvalid          IssueCode = "operand_invalid"
	IssueFormulaInvalid          IssueCode = "formula_invalid"
	IssueJSCompile               IssueCode = "js_compile"
	IssueLibraryRefMissing       IssueCode = "library_ref_missing"
	IssueLibraryUpdateAvailable  IssueCode = "library_update_available"
	IssueTopicMissing            IssueCode = "topic_missing"
	IssueExactlyOnceCrossCluster IssueCode = "exactly_once_cross_cluster"
	IssueTestCaseFailed          IssueCode = "test_case_failed"
	IssueLimitOutOfRange         IssueCode = "limit_out_of_range"
	IssueStepUnsupported         IssueCode = "step_unsupported"
	IssueFormatUnsupported       IssueCode = "format_unsupported"
	IssueVersionUnsupported      IssueCode = "version_unsupported"
)

// Issue is one validation finding addressed by a JSON Pointer into the document.
type Issue struct {
	Code     IssueCode `json:"code"`
	Severity Severity  `json:"severity"`
	Path     Pointer   `json:"path"`
	Message  string    `json:"message"`
}

// NewError builds an error-severity issue.
func NewError(code IssueCode, path Pointer, format string, args ...any) Issue {
	return Issue{Code: code, Severity: SeverityError, Path: path, Message: fmt.Sprintf(format, args...)}
}

// NewWarning builds a warning-severity issue.
func NewWarning(code IssueCode, path Pointer, format string, args ...any) Issue {
	return Issue{Code: code, Severity: SeverityWarning, Path: path, Message: fmt.Sprintf(format, args...)}
}

// Issues is an ordered collection of findings.
type Issues []Issue

// Sort orders issues by path, then code, keeping insertion order for ties.
func (is Issues) Sort() Issues {
	sort.SliceStable(is, func(i, j int) bool {
		if is[i].Path != is[j].Path {
			return is[i].Path < is[j].Path
		}
		return is[i].Code < is[j].Code
	})
	return is
}

// HasErrors reports whether any issue blocks deploy.
func (is Issues) HasErrors() bool {
	for _, i := range is {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Errors returns only error-severity issues.
func (is Issues) Errors() Issues {
	out := Issues{}
	for _, i := range is {
		if i.Severity == SeverityError {
			out = append(out, i)
		}
	}
	return out
}

// Merge returns a sorted union of this collection and the others.
func (is Issues) Merge(others ...Issues) Issues {
	out := append(Issues{}, is...)
	for _, o := range others {
		out = append(out, o...)
	}
	return out.Sort()
}

// Add appends one issue in place.
func (is *Issues) Add(i Issue) {
	*is = append(*is, i)
}
