package scheduler

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
)

// RolloutCoordinator advances rolling deployments one replica at a time (design 5.6): when every replica
// runs the version it starts the auto-rollback window (activated_at); an error within the window — or a
// failing replica while rolling — marks the deployment failed, and routes creates the auto_rollback version.
type RolloutCoordinator struct {
	Stores    state.Stores
	Lifecycle state.DeploymentLifecycle
	Timings   domain.Timings
	Now       func() time.Time
}

// Advance processes every rolling deployment.
func (c *RolloutCoordinator) Advance(ctx context.Context) error {
	rolling, err := c.Stores.Rollouts.Rolling(ctx)
	if err != nil {
		return err
	}
	for _, r := range rolling {
		if err := c.advance(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

// advance moves one deployment forward.
func (c *RolloutCoordinator) advance(ctx context.Context, r state.Rollout) error {
	rows, err := c.Stores.Assignments.ForRoute(ctx, r.RouteID)
	if err != nil || len(rows) == 0 {
		return err
	}
	progress := NewRolloutProgress(r.VersionID, rows)
	switch {
	case progress.Failed() != nil:
		return c.fail(ctx, r, progress.Failed())
	case r.ActivatedAt == nil && progress.Converged():
		return c.Stores.Rollouts.MarkActivated(ctx, r.DeploymentID, c.Now())
	case r.ActivatedAt == nil:
		return c.next(ctx, r, progress)
	case c.Now().Sub(*r.ActivatedAt) >= c.Timings.AutoRollback:
		return c.activate(ctx, r)
	}
	return nil
}

// next points one more replica at the version when none is in progress.
func (c *RolloutCoordinator) next(ctx context.Context, r state.Rollout, progress RolloutProgress) error {
	if progress.InProgress() {
		return nil
	}
	if row := progress.NextReplica(); row != nil {
		return c.Stores.Assignments.SetTarget(ctx, row.RouteID, row.WorkerID, r.VersionID)
	}
	return nil
}

// fail marks the deployment failed (routes rolls back, or pauses when there is no previous version).
func (c *RolloutCoordinator) fail(ctx context.Context, r state.Rollout, row *state.AssignmentRow) error {
	paused, err := c.Lifecycle.MarkFailed(ctx, r.DeploymentID, row.ErrorMessage)
	if err != nil || !paused {
		return err
	}
	h := domain.HaltReport{RouteID: r.RouteID, WorkerID: row.WorkerID, VersionID: r.VersionID, Reason: domain.HaltAutoRollbackFailed,
		ErrorCategory: row.ErrorCategory, Message: row.ErrorMessage}
	return c.Stores.Runtime.Halt(ctx, h, c.Now())
}

// activate marks the deployment active and supersedes the previously active one.
func (c *RolloutCoordinator) activate(ctx context.Context, r state.Rollout) error {
	if err := c.Lifecycle.MarkActive(ctx, r.DeploymentID); err != nil {
		return err
	}
	return c.Stores.Rollouts.SupersedeActive(ctx, r.RouteID, r.DeploymentID)
}

// RolloutProgress classifies the replicas of a route against a rolling version.
type RolloutProgress struct {
	version string
	rows    []state.AssignmentRow
}

// NewRolloutProgress builds the view.
func NewRolloutProgress(version string, rows []state.AssignmentRow) RolloutProgress {
	return RolloutProgress{version: version, rows: rows}
}

// Failed returns a replica reporting an error for the version.
func (p RolloutProgress) Failed() *state.AssignmentRow {
	for i := range p.rows {
		if p.rows[i].TargetVersionID == p.version && p.rows[i].FailedTarget() {
			return &p.rows[i]
		}
	}
	return nil
}

// Converged reports whether every replica runs the version.
func (p RolloutProgress) Converged() bool {
	for _, r := range p.rows {
		if r.TargetVersionID != p.version || !r.Converged() {
			return false
		}
	}
	return true
}

// InProgress reports whether a replica targets the version but does not run it yet.
func (p RolloutProgress) InProgress() bool {
	for _, r := range p.rows {
		if r.TargetVersionID == p.version && !r.Converged() {
			return true
		}
	}
	return false
}

// NextReplica returns the first replica still targeting another version.
func (p RolloutProgress) NextReplica() *state.AssignmentRow {
	for i := range p.rows {
		if p.rows[i].TargetVersionID != p.version {
			return &p.rows[i]
		}
	}
	return nil
}
