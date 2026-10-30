// Package jobs runs bulk dead-letter retry/discard as background jobs (dead-letters design §5.11):
// Postgres-backed queue claimed with FOR UPDATE SKIP LOCKED, paged through dlq.Manager, resumable by cursor.
package jobs

import (
	"encoding/json"
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
	StatusPendingApproval Status = "pending_approval"
	StatusRejected        Status = "rejected"
)

// Terminal reports whether the job will not change any more.
func (s Status) Terminal() bool {
	return s == StatusCancelled || s == StatusSucceeded || s == StatusFailed || s == StatusRejected
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
	ErrorTypes []record.ErrorType  `json:"errorTypes,omitempty"`
	Step       string              `json:"step,omitempty"`
	VersionNo  int                 `json:"versionNo,omitempty"`
	Text       string              `json:"q,omitempty"`
	Since      *time.Time          `json:"since,omitempty"`
	Until      *time.Time          `json:"until,omitempty"`
	Classes    []record.ErrorClass `json:"errorClass,omitempty"`
	Cause      string              `json:"fingerprint,omitempty"`
	IDs        []string            `json:"ids,omitempty"`
}

// MatchOf converts between the stored match and the domain query.
type MatchOf struct{}

// Query returns the domain query of m.
func (MatchOf) Query(m Match) dlq.Query {
	q := dlq.Query{ErrorTypes: m.ErrorTypes, Step: m.Step, VersionNo: m.VersionNo, Text: m.Text, Classes: m.Classes, Fingerprint: m.Cause}
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
	m := Match{ErrorTypes: q.ErrorTypes, Step: q.Step, VersionNo: q.VersionNo, Text: q.Text, Classes: q.Classes, Cause: q.Fingerprint}
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
	Rate        int                 `json:"ratePerSecond"`
	Note        string              `json:"note"`
	Edit        *dlq.RecordEdit     `json:"-"`
	Approval    Approval            `json:"approval"`
	Mine        *bool               `json:"requestedByMe,omitempty"`
}

// ForCaller marks whether user requested the job (the UI hides Approve/Reject on one's own replays).
func (j Job) ForCaller(user string) Job {
	mine := j.RequestedBy != "" && j.RequestedBy == user
	j.Mine = &mine
	return j
}

// Approval is the 4-eyes state of a replay job (Required: it waited or waits for a second person).
type Approval struct {
	Required     bool       `json:"required"`
	DecidedBy    string     `json:"decidedBy,omitempty"`
	DecidedAt    *time.Time `json:"decidedAt,omitempty"`
	DecisionNote string     `json:"decisionNote,omitempty"`
}

// JobView is the API shape of a job: ids are counted, never echoed, and the edit is only flagged.
type JobView struct {
	jobAlias
	IDsCount int  `json:"idsCount,omitempty"`
	Edited   bool `json:"edited"`
}

// jobAlias drops Job's MarshalJSON in JobView.
type jobAlias Job

// MarshalJSON renders the JobView.
func (j Job) MarshalJSON() ([]byte, error) {
	v := JobView{jobAlias: jobAlias(j), IDsCount: len(j.Match.IDs), Edited: j.Edit != nil}
	v.Match.IDs = nil
	return json.Marshal(v)
}

// Rate bounds the replay rate of a job (events per second).
type Rate int

const (
	DefaultRate Rate = 50
	MinRate     Rate = 1
	MaxRate     Rate = 1000
	MaxJobIDs   Rate = 10000
)

// Valid reports whether r is an accepted rate.
func (r Rate) Valid() bool { return r >= MinRate && r <= MaxRate }

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
