package dlq

import (
	"github.com/eventrouter/router/internal/platform/problem"
)

// WorkflowCode is the machine-readable reason of a workflow problem (problem `code`, design 00001 §10).
type WorkflowCode string

const (
	CodeApprovalRequired WorkflowCode = "replay_approval_required"
	CodeSelfApproval     WorkflowCode = "self_approval"
	CodeEditUnsupported  WorkflowCode = "edit_unsupported_format"
	CodeReplayDenied     WorkflowCode = "replay_not_allowed"
	CodeNotPending       WorkflowCode = "job_not_pending"
)

// CodedProblem is a classified problem with a reason code and an optional request pointer.
type CodedProblem struct {
	Kind   problem.Kind
	Detail string
	Code   WorkflowCode
	Path   string
}

func (e *CodedProblem) Error() string             { return string(e.Code) + ": " + e.Detail }
func (e *CodedProblem) ProblemKind() problem.Kind { return e.Kind }
func (e *CodedProblem) ProblemCode() string       { return string(e.Code) }
func (e *CodedProblem) ProblemPath() string       { return e.Path }
func (e *CodedProblem) Is(target error) bool {
	t, ok := target.(*CodedProblem)
	return ok && t.Code == e.Code
}
func (e *CodedProblem) At(path string) *CodedProblem { c := *e; c.Path = path; return &c }

// Workflow errors (edit, replay, approvals).
var (
	ErrApprovalRequired = &CodedProblem{Kind: problem.KindConflict, Code: CodeApprovalRequired,
		Detail: "Replaying to a production destination needs a second person. Request approval to replay these events."}
	ErrSelfApproval = &CodedProblem{Kind: problem.KindForbidden, Code: CodeSelfApproval,
		Detail: "You requested this replay, so someone else has to approve it."}
	ErrEditUnsupported = &CodedProblem{Kind: problem.KindValidation, Code: CodeEditUnsupported,
		Detail: "Events of Avro or Protobuf sources cannot be edited. Replay them unchanged or fix the route."}
	ErrReplayDenied = &CodedProblem{Kind: problem.KindForbidden, Code: CodeReplayDenied,
		Detail: "You can't replay events of this route: you need the Editor role in its team."}
	ErrJobNotPending = &CodedProblem{Kind: problem.KindConflict, Code: CodeNotPending,
		Detail: "This replay is not waiting for approval any more. Reload to see its status."}
	ErrInvalidEdit = problem.NewError(problem.KindMalformedBody, "The edited value does not match its encoding or the source format.")
	ErrEditTooBig  = problem.NewError(problem.KindValidation, "The edited event is larger than 1 MiB.")
	ErrInvalidRate = problem.NewError(problem.KindMalformedBody, "The replay rate must be between 1 and 1000 events per second.")
	ErrInvalidNote = problem.NewError(problem.KindValidation, "The note is longer than 2000 characters.")
)

// Workflow request paths.
const (
	PathClass       RequestPath = "/query/class"
	PathFingerprint RequestPath = "/query/fingerprint"
	PathMatchClass  RequestPath = "/match/errorClass"
	PathEdit        RequestPath = "/edit"
	PathEditKey     RequestPath = "/edit/key"
	PathEditValue   RequestPath = "/edit/value"
	PathEditHeaders RequestPath = "/edit/headers"
	PathRate        RequestPath = "/ratePerSecond"
	PathNote        RequestPath = "/note"
)
