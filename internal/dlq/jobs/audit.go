package jobs

import (
	"context"
	"log/slog"

	"github.com/eventrouter/router/internal/dlq"
)

// RouteLookup resolves a job's route for the audit object label and team (dlq.Manager.Route).
type RouteLookup interface {
	Route(ctx context.Context, id string) (dlq.RouteInfo, error)
}

// JobEndAuditDetail is the audited outcome of a background job: counts and filter, never payloads.
type JobEndAuditDetail struct {
	JobID       string              `json:"jobId"`
	Action      dlq.ActionKind      `json:"action"`
	Status      Status              `json:"status"`
	Processed   int                 `json:"processed"`
	Counts      map[dlq.Outcome]int `json:"counts"`
	Match       Match               `json:"match"`
	RequestedBy string              `json:"requestedBy"`
	Error       string              `json:"error,omitempty"`
}

// AuditNotifier records dlq.job_finished when a job ends, so bulk retries and discards that ran in the
// background reach the audit log, its export and the SIEM sinks like synchronous actions (DL-203).
type AuditNotifier struct {
	Trail  dlq.AuditTrail
	Routes RouteLookup
	Log    *slog.Logger
}

// JobEnded implements Notifier; failures are logged (the job state is already stored).
func (n AuditNotifier) JobEnded(ctx context.Context, j Job) {
	route, err := n.Routes.Route(ctx, j.RouteID)
	if err != nil {
		route = dlq.RouteInfo{ID: j.RouteID, Name: j.RouteName}
	}
	detail := JobEndAuditDetail{JobID: j.ID, Action: j.Action, Status: j.Status, Processed: j.Processed, Counts: j.Counts,
		Match: j.Match, RequestedBy: j.RequestedBy, Error: j.Error}
	if err := n.Trail.Record(ctx, dlq.AuditEvent{Action: dlq.ActionDLQJobFinished, Route: route, Detail: detail}); err != nil {
		n.Log.Warn(string(LogAuditFailed), string(LogAttrJob), j.ID, string(LogAttrError), err)
	}
}
