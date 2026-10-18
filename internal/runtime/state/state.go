// Package state is the control-plane persistence contract of route-runtime: worker, assignment, route
// demand, runtime and rollout records plus the store ports the internal API and the scheduler use.
// internal/store/runtimestore implements them on Postgres; statetest holds an in-memory fake.
package state

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/store"
)

// Worker is one row of workers plus its assignment count.
type Worker struct {
	ID              string    `json:"id"`
	Host            string    `json:"host"`
	Zone            string    `json:"zone"`
	Version         string    `json:"version"`
	Capacity        int       `json:"capacity"`
	CapacityUsed    float64   `json:"capacityUsed"`
	Draining        bool      `json:"draining"`
	LastHeartbeatAt time.Time `json:"lastHeartbeatAt"`
	StartedAt       time.Time `json:"startedAt"`
	Routes          int       `json:"routes"`
}

// Alive reports whether the worker heartbeated within ttl.
func (w Worker) Alive(now time.Time, ttl time.Duration) bool {
	return now.Sub(w.LastHeartbeatAt) <= ttl
}

// AssignmentRow is one lease with its rollout target and last reported replica status.
type AssignmentRow struct {
	RouteID           string
	WorkerID          string
	LeaseExpiresAt    time.Time
	TargetVersionID   string
	ReportedVersionID string
	RunnerState       domain.RunnerState
	ErrorCategory     domain.ErrorCategory
	ErrorMessage      string
	Lag               int64
	Partitions        int
	ReportedAt        *time.Time
	CreatedAt         time.Time
}

// Converged reports whether the replica runs its target version.
func (a AssignmentRow) Converged() bool {
	return a.TargetVersionID != "" && a.ReportedVersionID == a.TargetVersionID && a.RunnerState.Observed() == domain.ObservedRunning
}

// FailedTarget reports whether the replica reports an error for its target version.
func (a AssignmentRow) FailedTarget() bool {
	return a.TargetVersionID != "" && a.ReportedVersionID == a.TargetVersionID && a.RunnerState == domain.RunnerError
}

// RouteDemand is what the scheduler needs to know about one route.
type RouteDemand struct {
	RouteID         string
	Name            string
	DesiredState    string
	ActiveVersionID string
	Replicas        int
	Partitions      int
	UpdatedAt       time.Time
	HaltedAt        *time.Time
	Replaying       bool
}

// Halted reports whether the route is halted and not resumed or redeployed since.
func (d RouteDemand) Halted() bool {
	return d.HaltedAt != nil && !d.HaltLifted()
}

// HaltLifted reports whether the route was set running again after its halt (resume, deploy, rollback).
func (d RouteDemand) HaltLifted() bool {
	return d.HaltedAt != nil && d.DesiredState == string(DesiredRunning) && d.UpdatedAt.After(*d.HaltedAt)
}

// Schedulable reports whether the route should have running replicas (a replay suspends them).
func (d RouteDemand) Schedulable() bool {
	return d.DesiredState == string(DesiredRunning) && d.ActiveVersionID != "" && !d.Halted() && !d.Replaying
}

// DesiredState mirrors the route_desired_state values the scheduler reads.
type DesiredState string

const (
	DesiredRunning  DesiredState = "running"
	DesiredPaused   DesiredState = "paused"
	DesiredArchived DesiredState = "archived"
)

// RuntimeRow is one route_runtime row.
type RuntimeRow struct {
	RouteID          string
	Observed         domain.ObservedState
	ErrorMessage     string
	ErrorCategory    domain.ErrorCategory
	AssignedWorkers  []string
	RunningVersionID string
	RunningVersionNo int
	HaltedAt         *time.Time
	HaltReason       domain.HaltReason
	Lag              int64
	Partitions       int
	UpdatedAt        time.Time
}

// ReplicaView is one replica of the public runtime view.
type ReplicaView struct {
	WorkerID   string
	State      domain.RunnerState
	VersionNo  int
	Lag        int64
	ReportedAt *time.Time
	Error      string
}

// Rollout is one rolling_out deployment.
type Rollout struct {
	DeploymentID      string
	RouteID           string
	VersionID         string
	PreviousVersionID string
	ActivatedAt       *time.Time
}

// WorkerStore persists workers and their heartbeats.
type WorkerStore interface {
	UpsertHeartbeat(ctx context.Context, id string, hb domain.Heartbeat, now time.Time) error
	ExtendLeases(ctx context.Context, workerID string, until time.Time) error
	ListWorkers(ctx context.Context) ([]Worker, error)
	PageWorkers(ctx context.Context, limit int, after *store.Cursor) ([]Worker, *store.Cursor, error)
}

// AssignmentStore persists leases.
type AssignmentStore interface {
	ForWorker(ctx context.Context, workerID string) ([]domain.Assignment, error)
	ReportStatus(ctx context.Context, workerID string, replicas []domain.ReplicaStatus, now time.Time) error
	List(ctx context.Context) ([]AssignmentRow, error)
	ForRoute(ctx context.Context, routeID string) ([]AssignmentRow, error)
	Insert(ctx context.Context, a AssignmentRow) error
	Delete(ctx context.Context, routeID, workerID string) error
	DeleteForWorker(ctx context.Context, workerID string, routeIDs []string) error
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
	SetTarget(ctx context.Context, routeID, workerID, versionID string) error
}

// RouteStore reads routes and immutable versions.
type RouteStore interface {
	Demands(ctx context.Context) ([]RouteDemand, error)
	VersionConfig(ctx context.Context, versionID string) ([]byte, error)
	VersionNo(ctx context.Context, versionID string) (int, error)
	Exists(ctx context.Context, routeID string) (bool, error)
	ActiveVersion(ctx context.Context, routeID string) (string, error)
}

// RuntimeStore persists route_runtime.
type RuntimeStore interface {
	Get(ctx context.Context, routeID string) (RuntimeRow, error)
	Save(ctx context.Context, row RuntimeRow) error
	Halt(ctx context.Context, h domain.HaltReport, now time.Time) error
	ClearHalt(ctx context.Context, routeID string) error
}

// RolloutStore reads rolling_out deployments and stamps activation.
type RolloutStore interface {
	Rolling(ctx context.Context) ([]Rollout, error)
	MarkActivated(ctx context.Context, deploymentID string, at time.Time) error
	SupersedeActive(ctx context.Context, routeID, exceptDeploymentID string) error
}

// Replay is one route_replays row: a request to re-read a route's source from a timestamp.
type Replay struct {
	ID            string              `json:"id"`
	RouteID       string              `json:"routeId"`
	From          time.Time           `json:"from"`
	Status        domain.ReplayStatus `json:"status"`
	RequestedBy   string              `json:"requestedBy,omitempty"`
	RequestedAt   time.Time           `json:"requestedAt"`
	StartedAt     *time.Time          `json:"startedAt,omitempty"`
	FinishedAt    *time.Time          `json:"finishedAt,omitempty"`
	Error         string              `json:"error,omitempty"`
	ConsumerGroup string              `json:"consumerGroup,omitempty"`
	Partitions    int                 `json:"partitions"`
}

// ReplayStatusOf converts a stored replay_status value.
func ReplayStatusOf(s string) domain.ReplayStatus { return domain.ReplayStatus(s) }

// ReplayTransition moves a replay from one status to the next; the extra fields are written when set.
type ReplayTransition struct {
	ID         string
	From       domain.ReplayStatus
	To         domain.ReplayStatus
	At         time.Time
	Error      string
	Group      string
	Partitions int
}

// ReplayStore persists route_replays (at most one open replay per route).
type ReplayStore interface {
	Create(ctx context.Context, r Replay) (Replay, error)
	Get(ctx context.Context, routeID, id string) (Replay, error)
	Open(ctx context.Context) ([]Replay, error)
	Transition(ctx context.Context, t ReplayTransition) (bool, error)
}

// Stores bundles every port.
type Stores struct {
	Workers     WorkerStore
	Assignments AssignmentStore
	Routes      RouteStore
	Runtime     RuntimeStore
	Rollouts    RolloutStore
	Replays     ReplayStore
	Tx          store.TxManager
}

// DeploymentLifecycle is how route-runtime reports rollout outcomes to features/routes (in-process).
type DeploymentLifecycle interface {
	MarkActive(ctx context.Context, deploymentID string) error
	MarkFailed(ctx context.Context, deploymentID, reason string) (paused bool, err error)
}

// RoutePauser sets a halted route's desired state to paused, so a resume lifts the halt.
type RoutePauser interface {
	Pause(ctx context.Context, routeID string) error
}
