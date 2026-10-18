package approvals

import (
	"fmt"
	"strings"

	"github.com/eventrouter/router/internal/platform/problem"
)

// ProblemCode is the machine-readable `code` of approval problems.
type ProblemCode string

const (
	CodeAlreadyDecided ProblemCode = "already_decided"
	CodeSelfApproval   ProblemCode = "self_approval"
	CodeNoteRequired   ProblemCode = "note_required"
	CodeInvalidComment ProblemCode = "invalid_comment"
	CodeNotRequester   ProblemCode = "not_requester"
)

// CodedError is a classified approval error.
type CodedError struct {
	Kind   problem.Kind
	Code   ProblemCode
	Detail string
	Path   string
}

func (e *CodedError) Error() string             { return e.Detail }
func (e *CodedError) ProblemKind() problem.Kind { return e.Kind }
func (e *CodedError) ProblemCode() string       { return string(e.Code) }
func (e *CodedError) ProblemPath() string       { return e.Path }

var (
	// ErrNotPending is returned by the store when a decision finds the request no longer pending.
	ErrNotPending = &CodedError{Kind: problem.KindConflict, Code: CodeAlreadyDecided,
		Detail: "This request was already decided. Reload to see the decision."}
	// ErrSelfApproval enforces strict 4-eyes: nobody approves their own request, admins included (403).
	ErrSelfApproval = &CodedError{Kind: problem.KindForbidden, Code: CodeSelfApproval,
		Detail: "You requested this deploy, so someone else has to approve it."}
	// ErrNoteRequired is returned when a rejection has no note (422 at /note).
	ErrNoteRequired = &CodedError{Kind: problem.KindValidation, Code: CodeNoteRequired, Path: "/note",
		Detail: "Tell the requester why you reject this deploy."}
	// ErrInvalidComment is returned for empty or overlong comments (422 at /body).
	ErrInvalidComment = &CodedError{Kind: problem.KindValidation, Code: CodeInvalidComment, Path: "/body",
		Detail: "Write a comment of 1 to 4000 characters."}
	// ErrNotRequester is returned when someone else than the requester (or a workspace admin) withdraws (403).
	ErrNotRequester = &CodedError{Kind: problem.KindForbidden, Code: CodeNotRequester,
		Detail: "Only the person who requested this deploy (or a workspace admin) can withdraw it."}
)

// AlreadyDecidedError is the 409 naming the current status and who decided.
type AlreadyDecidedError struct {
	Status    ApprovalStatus
	DecidedBy string
}

func (e *AlreadyDecidedError) Error() string {
	label := strings.ReplaceAll(string(e.Status), "_", " ")
	if e.DecidedBy == "" {
		return fmt.Sprintf("This request is already %s. Reload to see the decision.", label)
	}
	return fmt.Sprintf("This request is already %s by %s. Reload to see the decision.", label, e.DecidedBy)
}
func (e *AlreadyDecidedError) ProblemKind() problem.Kind { return problem.KindConflict }
func (e *AlreadyDecidedError) ProblemCode() string       { return string(CodeAlreadyDecided) }

// Decided builds the 409 for a request that is no longer pending.
func Decided(a Approval) *AlreadyDecidedError {
	e := &AlreadyDecidedError{Status: a.Status}
	if a.DecidedBy != nil {
		e.DecidedBy = a.DecidedBy.Name
	}
	return e
}
