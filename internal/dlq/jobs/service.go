package jobs

import (
	"context"

	"github.com/eventrouter/router/internal/dlq"
)

// Preparer validates a job before it is queued (dlq.Manager.PrepareJob).
type Preparer interface {
	PrepareJob(ctx context.Context, kind dlq.ActionKind, routeID string) (dlq.RouteInfo, error)
}

// Service is the job façade used by HTTP: create, read, list, cancel (DL-112).
type Service struct {
	Repo     Repository
	Preparer Preparer
	Audit    dlq.AuditTrail
}

// JobAuditDetail is the audited detail of a job action (ids only, never payloads).
type JobAuditDetail struct {
	JobID  string         `json:"jobId"`
	Action dlq.ActionKind `json:"action"`
}

// Create queues a job over every unresolved dead letter of routeID matching q.
func (s Service) Create(ctx context.Context, kind dlq.ActionKind, actor dlq.Actor, routeID string, q dlq.Query) (Job, error) {
	route, err := s.Preparer.PrepareJob(ctx, kind, routeID)
	if err != nil {
		return Job{}, err
	}
	j, err := s.Repo.Create(ctx, NewJob{RouteID: route.ID, Action: kind, Match: MatchOf{}.Match(q), RequestedBy: actor.UserID})
	if err != nil {
		return Job{}, err
	}
	j.RouteName = route.Name
	return j, s.trail().Record(ctx, dlq.AuditEvent{Action: dlq.ActionDLQJobCreated, Route: route, Detail: JobAuditDetail{JobID: j.ID, Action: kind}})
}

// Get returns one job.
func (s Service) Get(ctx context.Context, id string) (Job, error) {
	return s.Repo.Get(ctx, id)
}

// Recent lists the newest jobs of a route.
func (s Service) Recent(ctx context.Context, routeID string) ([]Job, error) {
	return s.Repo.ListByRoute(ctx, routeID, int(RecentJobs))
}

// Cancel asks a job to stop (queued jobs stop at once).
func (s Service) Cancel(ctx context.Context, id string) (Job, error) {
	j, err := s.Repo.RequestCancel(ctx, id)
	if err != nil {
		return j, err
	}
	route := dlq.RouteInfo{ID: j.RouteID, Name: j.RouteName}
	return j, s.trail().Record(ctx, dlq.AuditEvent{Action: dlq.ActionDLQJobCancelled, Route: route, Detail: JobAuditDetail{JobID: j.ID, Action: j.Action}})
}

// trail returns the configured audit trail (none drops events).
func (s Service) trail() dlq.AuditTrail {
	if s.Audit == nil {
		return dlq.NoAuditTrail{}
	}
	return s.Audit
}
