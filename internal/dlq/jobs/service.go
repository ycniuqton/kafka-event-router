package jobs

import (
	"context"

	"github.com/eventrouter/router/internal/dlq"
)

// Preparer validates a job before it is queued (dlq.Manager.PrepareJob).
type Preparer interface {
	PrepareJob(ctx context.Context, kind dlq.ActionKind, routeID string) (dlq.RouteInfo, error)
}

// ReplayChecker is the strict 4-eyes gate of replays (dlq.Manager.CheckReplay); nil allows every replay.
type ReplayChecker interface {
	CheckReplay(ctx context.Context, route dlq.RouteInfo) (dlq.GateDecision, error)
}

// Service is the job façade used by HTTP: create, read, list, cancel (DL-112), approve/reject (00001 §4.5).
type Service struct {
	Repo     Repository
	Preparer Preparer
	Audit    dlq.AuditTrail
	Gate     ReplayChecker
}

// CreateRequest is a bulk job: a match or explicit ids, a rate limit and a note.
type CreateRequest struct {
	Match dlq.Query
	IDs   []string
	Rate  int
	Note  string
}

// ReplayAuditDetail is the audited detail of a replay request or decision (ids/paths only).
type ReplayAuditDetail struct {
	JobID    string          `json:"jobId"`
	Action   dlq.ActionKind  `json:"action"`
	IDsCount int             `json:"idsCount,omitempty"`
	Diff     []dlq.DiffEntry `json:"diff,omitempty"`
	Note     string          `json:"note,omitempty"`
}

// JobAuditDetail is the audited detail of a job action (ids only, never payloads).
type JobAuditDetail struct {
	JobID  string         `json:"jobId"`
	Action dlq.ActionKind `json:"action"`
}

// Create queues a job over every unresolved dead letter of routeID matching q.
func (s Service) Create(ctx context.Context, kind dlq.ActionKind, actor dlq.Actor, routeID string, q dlq.Query) (Job, error) {
	return s.CreateWith(ctx, kind, actor, routeID, CreateRequest{Match: q})
}

// CreateWith queues a job (ids or match, rate-limited); a retry the 4-eyes gate holds back waits for approval.
func (s Service) CreateWith(ctx context.Context, kind dlq.ActionKind, actor dlq.Actor, routeID string, req CreateRequest) (Job, error) {
	route, err := s.Preparer.PrepareJob(ctx, kind, routeID)
	if err != nil {
		return Job{}, err
	}
	pending, err := s.pending(ctx, kind, route)
	if err != nil {
		return Job{}, err
	}
	m := MatchOf{}.Match(req.Match)
	if len(req.IDs) > 0 {
		m = Match{IDs: req.IDs}
	}
	j, err := s.Repo.Create(ctx, NewJob{RouteID: route.ID, Action: kind, Match: m, RequestedBy: actor.UserID, Rate: req.Rate, Note: req.Note, Pending: pending})
	if err != nil {
		return Job{}, err
	}
	j.RouteName = route.Name
	action, detail := dlq.ActionDLQJobCreated, any(JobAuditDetail{JobID: j.ID, Action: kind})
	if pending {
		action, detail = dlq.ActionDLQReplayRequest, ReplayAuditDetail{JobID: j.ID, Action: kind, IDsCount: len(req.IDs), Note: req.Note}
	}
	return j, s.trail().Record(ctx, dlq.AuditEvent{Action: action, Route: route, Detail: detail})
}

// pending asks the gate whether a retry job must wait for approval (discard never replays anything).
func (s Service) pending(ctx context.Context, kind dlq.ActionKind, route dlq.RouteInfo) (bool, error) {
	if kind != dlq.ActionRetry || s.Gate == nil {
		return false, nil
	}
	d, err := s.Gate.CheckReplay(ctx, route)
	if err != nil {
		return false, err
	}
	if d == dlq.GateDeny {
		return false, dlq.ErrReplayDenied
	}
	return d == dlq.GateRequireApproval, nil
}

// CreatePending parks a single (edited) replay the gate held back; the planned edit waits in the job.
func (s Service) CreatePending(ctx context.Context, actor dlq.Actor, p dlq.PendingReplay) (Job, error) {
	j, err := s.Repo.Create(ctx, NewJob{RouteID: p.Route.ID, Action: dlq.ActionRetry, Match: Match{IDs: []string{p.ID}},
		RequestedBy: actor.UserID, Rate: int(DefaultRate), Note: p.Note, Edit: p.Edit, Pending: true})
	if err != nil {
		return Job{}, err
	}
	j.RouteName = p.Route.Name
	detail := ReplayAuditDetail{JobID: j.ID, Action: dlq.ActionRetry, IDsCount: 1, Diff: p.Diff, Note: p.Note}
	return j, s.trail().Record(ctx, dlq.AuditEvent{Action: dlq.ActionDLQReplayRequest, Route: p.Route, Detail: detail})
}

// Decide approves or rejects a pending replay; the requester never decides their own (admins included).
func (s Service) Decide(ctx context.Context, id string, d Decision) (Job, error) {
	j, err := s.Repo.Get(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if j.RequestedBy != "" && j.RequestedBy == d.By {
		return Job{}, dlq.ErrSelfApproval
	}
	if j, err = s.Repo.Decide(ctx, id, d); err != nil {
		return j, err
	}
	action := dlq.ActionDLQReplayApproved
	if !d.Approve {
		action = dlq.ActionDLQReplayRejected
	}
	route := dlq.RouteInfo{ID: j.RouteID, Name: j.RouteName}
	return j, s.trail().Record(ctx, dlq.AuditEvent{Action: action, Route: route, Detail: ReplayAuditDetail{JobID: j.ID, Action: j.Action, Note: d.Note}})
}

// Pending lists replays waiting for approval (oldest first).
func (s Service) Pending(ctx context.Context) ([]Job, error) {
	return s.Repo.ListPending(ctx, int(RecentJobs)*5)
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
