package approvals

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// ApprovalStatus is the deployment status of an approval request (Postgres enum deployment_status).
type ApprovalStatus string

const (
	StatusPending    ApprovalStatus = "pending_approval"
	StatusRollingOut ApprovalStatus = "rolling_out"
	StatusActive     ApprovalStatus = "active"
	StatusFailed     ApprovalStatus = "failed"
	StatusSuperseded ApprovalStatus = "superseded"
	StatusRejected   ApprovalStatus = "rejected"
	StatusCancelled  ApprovalStatus = "cancelled"
)

// Statuses is the catalogue of approval statuses (filter parsing).
type Statuses struct{}

// Valid reports whether s is a known status.
func (Statuses) Valid(s ApprovalStatus) bool {
	switch s {
	case StatusPending, StatusRollingOut, StatusActive, StatusFailed, StatusSuperseded, StatusRejected, StatusCancelled:
		return true
	}
	return false
}

// CancelReason tells why a pending request was cancelled.
type CancelReason string

const (
	CancelWithdrawn  CancelReason = "withdrawn"
	CancelSuperseded CancelReason = "superseded"
)

// Person is a user shown on an approval (requester, decider, comment author).
type Person struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Destination is one branch destination of the requested version.
type Destination struct {
	ConnectionID string      `json:"connectionId"`
	Name         string      `json:"name"`
	EnvTag       rbac.EnvTag `json:"envTag"`
}

// Approval is one approval request (a deployment that needed a second person).
type Approval struct {
	ID           uuid.UUID        `json:"id"`
	RouteID      uuid.UUID        `json:"routeId"`
	RouteName    string           `json:"routeName"`
	TeamID       rbac.TeamID      `json:"teamId"`
	VersionID    uuid.UUID        `json:"versionId"`
	VersionNo    int              `json:"versionNo"`
	Status       ApprovalStatus   `json:"status"`
	RequestedBy  *Person          `json:"requestedBy"`
	RequestedAt  time.Time        `json:"requestedAt"`
	ChangeNote   string           `json:"changeNote"`
	DecidedBy    *Person          `json:"decidedBy"`
	DecidedAt    *time.Time       `json:"decidedAt"`
	DecisionNote string           `json:"decisionNote"`
	CancelReason CancelReason     `json:"cancelReason,omitempty"`
	Destinations []Destination    `json:"destinations"`
	RowVersion   store.RowVersion `json:"rowVersion"`
}

// IsPending reports whether the request still waits for a decision.
func (a Approval) IsPending() bool {
	return a.Status == StatusPending
}

// RequestedByUser reports whether user requested the deploy.
func (a Approval) RequestedByUser(user uuid.UUID) bool {
	return a.RequestedBy != nil && a.RequestedBy.ID == user
}

// Comment is one review comment.
type Comment struct {
	ID         uuid.UUID `json:"id"`
	ApprovalID uuid.UUID `json:"approvalId"`
	Author     *Person   `json:"author"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"createdAt"`
}

// VersionConfig is a route version's number and configuration (review diff input).
type VersionConfig struct {
	No     int             `json:"no"`
	Config json.RawMessage `json:"config"`
}

// ApprovalDetail is the review page: the request, both configurations, comments and what the caller may do.
type ApprovalDetail struct {
	Approval      Approval       `json:"approval"`
	Requested     VersionConfig  `json:"requested"`
	Active        *VersionConfig `json:"active"`
	Comments      []Comment      `json:"comments"`
	CanDecide     bool           `json:"canDecide"`
	CanWithdraw   bool           `json:"canWithdraw"`
	EligibleCount int            `json:"eligibleApprovers"`
}

// ListFilter narrows approval lists; Mine means requested by the caller.
type ListFilter struct {
	Status  *ApprovalStatus
	TeamID  *rbac.TeamID
	RouteID *uuid.UUID
	Mine    bool
	Caller  uuid.UUID
}

// Page is one keyset page request (newest first).
type Page struct {
	Limit int
	After *store.Cursor
}

// GateOutcome is what a deploy must do.
type GateOutcome string

const (
	OutcomeDeployNow       GateOutcome = "deploy_now"
	OutcomeRequireApproval GateOutcome = "require_approval"
	OutcomeDeny            GateOutcome = "deny"
)

// GateReason explains the outcome.
type GateReason string

const (
	ReasonNonProd           GateReason = "non_prod"
	ReasonProdDirect        GateReason = "prod_direct"
	ReasonProdApproval      GateReason = "prod_requires_approval"
	ReasonMissingPermission GateReason = "missing_permission"
	ReasonNoApprover        GateReason = "no_approver"
)

// GateResult is the gate's answer; Approvers counts eligible approvers other than the caller.
type GateResult struct {
	Outcome   GateOutcome `json:"outcome"`
	Reason    GateReason  `json:"reason"`
	Approvers int         `json:"approvers"`
}

// DeployTarget is what a deploy would make live: the route's team and every destination connection.
type DeployTarget struct {
	RouteID      uuid.UUID
	TeamID       rbac.TeamID
	Destinations []rbac.Resource
}

// HasProd reports whether any destination is tagged prod.
func (t DeployTarget) HasProd() bool {
	for _, d := range t.Destinations {
		if d.EnvTag == rbac.EnvProd {
			return true
		}
	}
	return false
}

// TeamPolicy is the team's approval setting.
type TeamPolicy struct {
	RequireProdApproval bool
}

// CallerRights are the caller's deploy permissions on the route's team.
type CallerRights struct {
	DeployNonProd bool
	DeployProd    bool
	Write         bool
}

// RequestInput is what routes passes after creating the pending deployment (inside its deploy tx).
type RequestInput struct {
	DeploymentID uuid.UUID
	RouteID      uuid.UUID
}

// EventKind names approval notifications.
type EventKind string

const (
	EventRequested EventKind = "approval.requested"
	EventDecided   EventKind = "approval.decided"
	EventCommented EventKind = "approval.commented"
	EventWithdrawn EventKind = "approval.withdrawn"
)

// ApprovalEvent is one notification with its recipients (bell entries link to the review page).
type ApprovalEvent struct {
	Kind       EventKind      `json:"kind"`
	ApprovalID uuid.UUID      `json:"approvalId"`
	RouteID    uuid.UUID      `json:"routeId"`
	RouteName  string         `json:"routeName"`
	TeamID     rbac.TeamID    `json:"teamId"`
	VersionNo  int            `json:"versionNo"`
	Status     ApprovalStatus `json:"status"`
	Actor      uuid.UUID      `json:"actor"`
	Note       string         `json:"note,omitempty"`
	Recipients []uuid.UUID    `json:"recipients"`
	At         time.Time      `json:"at"`
}
