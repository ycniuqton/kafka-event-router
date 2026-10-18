package routes

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// RouteID identifies a route (uuid text).
type RouteID string

// VersionID identifies an immutable route version.
type VersionID string

// DeploymentID identifies one deployment attempt.
type DeploymentID string

// TeamID identifies the owning team.
type TeamID string

// Revision is routes.row_version, the If-Match / ETag token of a route.
type Revision int64

// Route is the stored route row: identity, settings, desired state and the editable draft.
type Route struct {
	ID              RouteID
	TeamID          TeamID
	Name            string
	Description     string
	Tags            []string
	DesiredState    DesiredState
	ActiveVersionID *VersionID
	Replicas        int
	Draft           json.RawMessage
	Revision        Revision
	CreatedBy       *uuid.UUID
	UpdatedBy       *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// HasActiveVersion reports whether the route was deployed at least once.
func (r Route) HasActiveVersion() bool {
	return r.ActiveVersionID != nil
}

// RouteVersion is an immutable deployed configuration.
type RouteVersion struct {
	ID         VersionID       `json:"id"`
	RouteID    RouteID         `json:"routeId"`
	No         int             `json:"no"`
	Config     json.RawMessage `json:"config"`
	ConfigHash string          `json:"configHash"`
	Origin     VersionOrigin   `json:"origin"`
	BasedOn    *VersionID      `json:"basedOnId,omitempty"`
	BasedOnNo  *int            `json:"basedOn,omitempty"`
	CreatedBy  *uuid.UUID      `json:"-"`
	Author     *UserRef        `json:"author"`
	ChangeNote string          `json:"changeNote"`
	CreatedAt  time.Time       `json:"createdAt"`
}

// Deployment is one attempt to make a version live.
type Deployment struct {
	ID                DeploymentID     `json:"id"`
	RouteID           RouteID          `json:"routeId"`
	VersionID         VersionID        `json:"versionId"`
	VersionNo         int              `json:"versionNo"`
	PreviousVersionID *VersionID       `json:"previousVersionId,omitempty"`
	Status            DeploymentStatus `json:"status"`
	FailureReason     string           `json:"failureReason,omitempty"`
	RequestedBy       *uuid.UUID       `json:"requestedBy,omitempty"`
	ApprovedBy        *uuid.UUID       `json:"approvedBy,omitempty"`
	RequestedAt       time.Time        `json:"requestedAt"`
	ApprovedAt        *time.Time       `json:"approvedAt,omitempty"`
	StartedAt         *time.Time       `json:"startedAt,omitempty"`
	FinishedAt        *time.Time       `json:"finishedAt,omitempty"`
}

// UserRef is a user shown next to a change.
type UserRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ActiveVersionRef points at the live version of a route.
type ActiveVersionRef struct {
	ID         VersionID       `json:"id"`
	No         int             `json:"no"`
	ConfigHash string          `json:"-"`
	Config     json.RawMessage `json:"-"`
}

// SummaryRow is what the repository returns for list and detail reads.
type SummaryRow struct {
	Route         Route
	TeamName      string
	ObservedState ObservedState
	Active        *ActiveVersionRef
	UpdatedBy     *UserRef
}

// RouteChange describes a settings / draft update requested through PATCH.
type RouteChange struct {
	Name        *string
	Description *string
	Tags        *[]string
	Replicas    *int
	Draft       json.RawMessage
}

// IsEmpty reports whether the change touches nothing.
func (c RouteChange) IsEmpty() bool {
	return c.Name == nil && c.Description == nil && c.Tags == nil && c.Replicas == nil && c.Draft == nil
}

// NewRoute is the input of RouteManager.Create.
type NewRoute struct {
	Name        string
	TeamID      TeamID
	Description string
	Tags        []string
	Draft       json.RawMessage
}

// Actor is who performs a manager operation (nil user for system / workload callers).
type Actor struct {
	UserID  *uuid.UUID
	Teams   map[TeamID]bool
	Ordered []TeamID
	System  bool
}

// DefaultTeam is the first team the actor may write to, if any.
func (a Actor) DefaultTeam() (TeamID, bool) {
	if len(a.Ordered) == 0 {
		return "", false
	}
	return a.Ordered[0], true
}
