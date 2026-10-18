package rbac

import (
	"fmt"
	"strings"

	"github.com/eventrouter/router/internal/platform/problem"
)

// ProblemCode is the machine-readable `code` member of access-control problems.
type ProblemCode string

const (
	CodeLastAdmin          ProblemCode = "last_admin"
	CodeRoleNotAssignable  ProblemCode = "role_not_assignable"
	CodeInvalidDisplayName ProblemCode = "invalid_display_name"
	CodeTeamNotEmpty       ProblemCode = "team_not_empty"
	CodeInvalidTeamName    ProblemCode = "invalid_team_name"
	CodeTeamNameTaken      ProblemCode = "team_name_taken"
	CodeInvalidEmail       ProblemCode = "invalid_email"
	CodeUserExists         ProblemCode = "user_exists"
	CodeGrantToOwner       ProblemCode = "grant_to_owner"
	CodeUnknownConnection  ProblemCode = "unknown_connection"
	CodeUnknownTeam        ProblemCode = "unknown_team"
)

// CodedError is a classified access-control error with a reason code and optional JSON Pointer.
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
	// ErrLastAdmin is returned when a change would leave the workspace (or a team) without an admin (409).
	ErrLastAdmin = &CodedError{Kind: problem.KindConflict, Code: CodeLastAdmin,
		Detail: "This would leave no administrator. Make someone else an admin first, then retry."}
	// ErrRoleNotAssignable is returned for roles not assignable in the current phase (422 at /role).
	ErrRoleNotAssignable = &CodedError{Kind: problem.KindValidation, Code: CodeRoleNotAssignable, Path: "/role",
		Detail: "This role can't be assigned in this workspace. Pick one of the offered roles."}
	// ErrInvalidDisplayName is returned for empty or overlong display names (422 at /displayName).
	ErrInvalidDisplayName = &CodedError{Kind: problem.KindValidation, Code: CodeInvalidDisplayName, Path: "/displayName",
		Detail: "Enter a display name of 1 to 200 characters."}
	// ErrInvalidTeamName is returned for empty or overlong team names (422 at /name).
	ErrInvalidTeamName = &CodedError{Kind: problem.KindValidation, Code: CodeInvalidTeamName, Path: "/name",
		Detail: "Enter a team name of 1 to 100 characters."}
	// ErrTeamNameTaken is returned when another team already uses the name (409 at /name).
	ErrTeamNameTaken = &CodedError{Kind: problem.KindConflict, Code: CodeTeamNameTaken, Path: "/name",
		Detail: "Another team already uses this name. Pick a different name."}
	// ErrInvalidEmail is returned for invitations without a usable email (422 at /email).
	ErrInvalidEmail = &CodedError{Kind: problem.KindValidation, Code: CodeInvalidEmail, Path: "/email",
		Detail: "Enter a valid email address, like name@company.com."}
	// ErrUserExists is returned when an invitation targets an email that already has a user (409 at /email).
	ErrUserExists = &CodedError{Kind: problem.KindConflict, Code: CodeUserExists, Path: "/email",
		Detail: "Someone with this email is already a member. Change their role in the member list instead."}
	// ErrGrantToOwner is returned when a team is granted its own connection (422 at /connectionId).
	ErrGrantToOwner = &CodedError{Kind: problem.KindValidation, Code: CodeGrantToOwner, Path: "/connectionId",
		Detail: "This team already owns the connection, so it needs no grant."}
	// ErrUnknownConnection is returned when a grant names a connection that does not exist (404).
	ErrUnknownConnection = &CodedError{Kind: problem.KindNotFound, Code: CodeUnknownConnection, Path: "/connectionId",
		Detail: "This connection does not exist. It may have been deleted."}
	// ErrUnknownTeam is returned when a command names a team that does not exist (422 at /teamId).
	ErrUnknownTeam = &CodedError{Kind: problem.KindValidation, Code: CodeUnknownTeam, Path: "/teamId",
		Detail: "This team does not exist. Pick an existing team."}
)

// TeamNotEmptyError is the 409 returned when a team still owns objects; it names what blocks the delete.
type TeamNotEmptyError struct {
	Usage TeamUsage
}

func (e *TeamNotEmptyError) Error() string {
	return fmt.Sprintf("This team still owns %d routes, %d connections, %d transforms and %d lookup tables. Move or delete them first.",
		e.Usage.Routes, e.Usage.Connections, e.Usage.Transforms, e.Usage.LookupTables)
}
func (e *TeamNotEmptyError) ProblemKind() problem.Kind { return problem.KindConflict }
func (e *TeamNotEmptyError) ProblemCode() string       { return string(CodeTeamNotEmpty) }

// ForbiddenError is the 403 returned for denied decisions; it names the missing permission.
type ForbiddenError struct {
	Decision Decision
	Detail   string
}

func (e *ForbiddenError) Error() string             { return e.Detail }
func (e *ForbiddenError) ProblemKind() problem.Kind { return problem.KindForbidden }
func (e *ForbiddenError) ProblemCode() string       { return string(e.Decision.Reason) }
func (e *ForbiddenError) MissingPermission() string { return string(e.Decision.Missing) }

// ForbiddenMessages turns denied decisions into "what / why / what to do" sentences.
type ForbiddenMessages struct {
	matrix RoleMatrix
}

// NewForbiddenMessages builds the message source over the role matrix.
func NewForbiddenMessages(matrix RoleMatrix) ForbiddenMessages {
	return ForbiddenMessages{matrix: matrix}
}

// Error builds the ForbiddenError for d.
func (m ForbiddenMessages) Error(d Decision) *ForbiddenError {
	return &ForbiddenError{Decision: d, Detail: m.detail(d)}
}

func (m ForbiddenMessages) detail(d Decision) string {
	switch d.Reason {
	case ReasonDisabledUser:
		return "Your account is disabled. Ask a workspace admin to enable it."
	case ReasonEngineError:
		return "We couldn't check your permissions right now. Retry in a moment."
	case ReasonNoBinding:
		return "You are not a member of any team yet. Ask an admin to add you to a team."
	}
	role, ok := m.matrix.LowestRoleWith(d.Missing)
	if !ok {
		return "Only workspace admins can do this. Ask a workspace admin."
	}
	return fmt.Sprintf("You need the %s role in this team to do this. Ask an admin to change your role.", RoleLabel(role))
}

// RoleLabel returns the display name of a role ("editor" → "Editor").
func RoleLabel(r Role) string {
	s := string(r)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
