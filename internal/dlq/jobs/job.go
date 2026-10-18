// Package jobs runs bulk dead-letter retry/discard as background jobs (dead-letters design §5.11):
// Postgres-backed queue claimed with FOR UPDATE SKIP LOCKED, paged through dlq.Manager, resumable by cursor.
package jobs

import (
	"time"

	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/record"
)

// Status is the lifecycle state of a job (dlq_job_status).
type Status string

const (
	StatusQueued          Status = "queued"
	StatusRunning         Status = "running"
	StatusCancelRequested Status = "cancel_requested"
	StatusCancelled       Status = "cancelled"
	StatusSucceeded       Status = "succeeded"
	StatusFailed          Status = "failed"
)

// Terminal reports whether the job will not change any more.
func (s Status) Terminal() bool {
	return s == StatusCancelled || s == StatusSucceeded || s == StatusFailed
}

// Timing holds the runner's intervals.
type Timing time.Duration

const (
	TickInterval      Timing = Timing(2 * time.Second)
	ReapInterval      Timing = Timing(60 * time.Second)
	StaleAfter        Timing = Timing(60 * time.Second)
	HeartbeatInterval Timing = Timing(15 * time.Second)
)

// Duration returns the interval as a time.Duration.
func (t Timing) Duration() time.Duration { return time.Duration(t) }

// ListLimit bounds the recent jobs listed per route.
type ListLimit int

const RecentJobs ListLimit = 20

// Match is the stored filter of a job (selection jsonb).
type Match struct {
	ErrorTypes []record.ErrorType `json:"errorTypes,omitempty"`
	Step       string             `json:"step,omitempty"`
	VersionNo  int                `json:"versionNo,omitempty"`
	Text       string             `json:"q,omitempty"`
	Since      *time.Time         `json:"since,omitempty"`
	Until      *time.Time         `json:"until,omitempty"`
}

// MatchOf converts between the stored match and the domain query.
type MatchOf struct{}

// Query returns the domain query of m.
func (MatchOf) Query(m Match) dlq.Query {
	q := dlq.Query{ErrorTypes: m.ErrorTypes, Step: m.Step, VersionNo: m.VersionNo, Text: m.Text}
	if m.Since != nil {
		q.Since = *m.Since
	}
	if m.Until != nil {
		q.Until = *m.Until
	}
	return q
}

// Match returns the stored form of q.
func (MatchOf) Match(q dlq.Query) Match {
	m := Match{ErrorTypes: q.ErrorTypes, Step: q.Step, VersionNo: q.VersionNo, Text: q.Text}
	if !q.Since.IsZero() {
		m.Since = &q.Since
	}
	if !q.Until.IsZero() {
		m.Until = &q.Until
	}
	return m
}

// Job is one bulk action over every dead letter of a route matching a filter.
type Job struct {
	ID          string              `json:"jobId"`
	RouteID     string              `json:"routeId"`
	RouteName   string              `json:"routeName"`
	Action      dlq.ActionKind      `json:"action"`
	Match       Match               `json:"match"`
	Status      Status              `json:"status"`
	Total       int                 `json:"total"`
	Processed   int                 `json:"processed"`
	Counts      map[dlq.Outcome]int `json:"counts"`
	Cursor      string              `json:"-"`
	Error       string              `json:"error,omitempty"`
	RequestedBy string              `json:"requestedBy"`
	HeartbeatAt *time.Time          `json:"-"`
	CreatedAt   time.Time           `json:"createdAt"`
	StartedAt   *time.Time          `json:"startedAt"`
	FinishedAt  *time.Time          `json:"finishedAt"`
}

// Apply adds the results of one page to the job's progress.
func (j *Job) Apply(page dlq.JobPage) {
	if j.Counts == nil {
		j.Counts = map[dlq.Outcome]int{}
	}
	for _, res := range page.Report.Results {
		j.Counts[res.Outcome]++
	}
	j.Processed += len(page.Report.Results)
	j.Cursor = page.Cursor
	j.Total = max(j.Total, j.Processed)
}
