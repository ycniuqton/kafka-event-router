package jobs

import (
	"context"
	"encoding/json"
	"log/slog"
)

// EventType names the push events of finished jobs (monitoring events bus, bell + toast).
type EventType string

const (
	EventJobFinished EventType = "dlq.job.finished"
	EventJobFailed   EventType = "dlq.job.failed"
)

// EventOf maps a terminal status to its push event (cancelled jobs finish too).
var EventOf = map[Status]EventType{StatusSucceeded: EventJobFinished, StatusCancelled: EventJobFinished, StatusFailed: EventJobFailed}

// Notifier tells users that a job ended (DL-113).
type Notifier interface {
	JobEnded(ctx context.Context, j Job)
}

// Publisher is the port to the monitoring events bus (monitoring#P2-BE-14 implements it); the event
// is addressed to the job's requester.
type Publisher interface {
	Publish(ctx context.Context, eventType string, userID string, payload json.RawMessage) error
}

// BusNotifier publishes job ends on the events bus.
type BusNotifier struct {
	Bus Publisher
	Log *slog.Logger
}

// JobEnded implements Notifier; publish errors are logged (the job state is already stored).
func (n BusNotifier) JobEnded(ctx context.Context, j Job) {
	payload, err := json.Marshal(j)
	if err == nil {
		err = n.Bus.Publish(ctx, string(EventOf[j.Status]), j.RequestedBy, payload)
	}
	if err != nil {
		n.Log.Warn(string(LogPublishFailed), string(LogAttrJob), j.ID, string(LogAttrError), err)
	}
}

// LogNotifier logs job ends (ids and counts only, never payloads).
type LogNotifier struct {
	Log *slog.Logger
}

// JobEnded implements Notifier.
func (n LogNotifier) JobEnded(_ context.Context, j Job) {
	n.Log.Info(string(EventOf[j.Status]), string(LogAttrJob), j.ID, string(LogAttrRoute), j.RouteID,
		string(LogAttrStatus), string(j.Status), string(LogAttrProcessed), j.Processed)
}

// Notifiers fans a job end out to every notifier.
type Notifiers []Notifier

// JobEnded implements Notifier.
func (ns Notifiers) JobEnded(ctx context.Context, j Job) {
	for _, n := range ns {
		n.JobEnded(ctx, j)
	}
}

// LogMessage names log messages of the job runner.
type LogMessage string

const (
	LogPublishFailed LogMessage = "dlq job: publish failed"
	LogRunnerError   LogMessage = "dlq job: runner error"
	LogRequeued      LogMessage = "dlq job: requeued stale jobs"
	LogAuditFailed   LogMessage = "dlq job: audit failed"
)

// LogAttr names log attributes of the job runner.
type LogAttr string

const (
	LogAttrJob       LogAttr = "job_id"
	LogAttrRoute     LogAttr = "route_id"
	LogAttrStatus    LogAttr = "status"
	LogAttrProcessed LogAttr = "processed"
	LogAttrError     LogAttr = "error"
	LogAttrCount     LogAttr = "count"
)
