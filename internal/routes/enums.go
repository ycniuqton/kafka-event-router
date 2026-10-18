package routes

// DesiredState mirrors the route_desired_state enum: what the operator wants the runners to do.
type DesiredState string

const (
	DesiredRunning  DesiredState = "running"
	DesiredPaused   DesiredState = "paused"
	DesiredArchived DesiredState = "archived"
)

// ObservedState mirrors route_observed_state, written by route-runtime; empty means unknown.
type ObservedState string

const (
	ObservedUnknown ObservedState = ""
	ObservedRunning ObservedState = "running"
	ObservedPaused  ObservedState = "paused"
	ObservedError   ObservedState = "error"
)

// Status is the derived badge shown in the UI.
type Status string

const (
	StatusDraft    Status = "draft"
	StatusRunning  Status = "running"
	StatusPaused   Status = "paused"
	StatusError    Status = "error"
	StatusArchived Status = "archived"
)

// AllStatuses lists every derived status in display order.
var AllStatuses = []Status{StatusDraft, StatusRunning, StatusPaused, StatusError, StatusArchived}

// VersionOrigin mirrors route_version_origin: why a version row exists.
type VersionOrigin string

const (
	OriginDeploy       VersionOrigin = "deploy"
	OriginRollback     VersionOrigin = "rollback"
	OriginAutoRollback VersionOrigin = "auto_rollback"
	OriginImport       VersionOrigin = "import"
)

// DeploymentStatus mirrors deployment_status (rejected/cancelled arrive with access-control approvals).
type DeploymentStatus string

const (
	DeployPendingApproval DeploymentStatus = "pending_approval"
	DeployRollingOut      DeploymentStatus = "rolling_out"
	DeployActive          DeploymentStatus = "active"
	DeployFailed          DeploymentStatus = "failed"
	DeploySuperseded      DeploymentStatus = "superseded"
	DeployRejected        DeploymentStatus = "rejected"
	DeployCancelled       DeploymentStatus = "cancelled"
)

// OpenDeploymentStatuses are the statuses a newer deploy supersedes.
var OpenDeploymentStatuses = []DeploymentStatus{DeployPendingApproval, DeployRollingOut}

// TransformIndicator summarises a route's transform for the list column.
type TransformIndicator string

const (
	TransformNone   TransformIndicator = "none"
	TransformVisual TransformIndicator = "visual"
	TransformJS     TransformIndicator = "js"
)

// BulkAction names an action of POST /routes/bulk.
type BulkAction string

const (
	BulkPause  BulkAction = "pause"
	BulkResume BulkAction = "resume"
	BulkAddTag BulkAction = "add_tag"
	BulkExport BulkAction = "export"
)

// DocumentFormat names a serialisation of route documents (YAML export/import is P2).
type DocumentFormat string

const (
	FormatJSON DocumentFormat = "json"
	FormatYAML DocumentFormat = "yaml"
)

// VersionRefKind names the symbolic version references accepted by the diff endpoint.
type VersionRefKind string

const (
	RefDraft  VersionRefKind = "draft"
	RefActive VersionRefKind = "active"
)

// RouteLimit bounds route-level request values.
type RouteLimit int

const (
	MaxBulkIDs     RouteLimit = 200
	MaxNameLength  RouteLimit = 120
	MaxTagLength   RouteLimit = 64
	DefaultReplica RouteLimit = 2
	MaxReplicas    RouteLimit = 64
)
