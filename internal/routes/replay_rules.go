package routes

import (
	"fmt"
	"time"

	"github.com/eventrouter/router/internal/platform/api"
)

// Request members of POST /routes/{id}/replay addressed by validation errors.
const (
	PathConfirm ErrorPath = "/confirm"
	PathFrom    ErrorPath = "/from"
)

// Validation codes of replay requests.
const (
	CodeConfirmRequired RuleCode = "confirm_required"
	CodeInFuture        RuleCode = "in_future"
	CodeBeyondRetention RuleCode = "beyond_retention"
)

// ReplayWindow bounds how far back a replay may start (the source topics' retention).
type ReplayWindow time.Duration

// DefaultReplayWindow is Kafka's default log retention (log.retention.hours = 168).
const DefaultReplayWindow = ReplayWindow(168 * time.Hour)

// Day is the unit replay windows are shown in.
const Day ReplayWindow = ReplayWindow(24 * time.Hour)

// Days is the window in whole days.
func (w ReplayWindow) Days() int { return int(w / Day) }

// ReplayRules validates replay requests: an explicit confirmation that the destination receives
// duplicates, and a start time in the past but inside the retention window.
type ReplayRules struct {
	Window ReplayWindow
}

// Check returns a 422 validation error listing every problem, or nil.
func (r ReplayRules) Check(req ReplayRequest, now time.Time) error {
	v := api.NewValidationError()
	if !req.Confirm {
		v.Add(string(PathConfirm), string(CodeConfirmRequired), "Confirm the replay: the destination will receive duplicates of events it already got.")
	}
	switch {
	case req.From == nil || req.From.IsZero():
		v.Add(string(PathFrom), string(CodeRequired), "Pick the date and time to replay from.")
	case req.From.After(now):
		v.Add(string(PathFrom), string(CodeInFuture), "The replay start is in the future. Pick a time in the past.")
	case now.Sub(*req.From) > time.Duration(r.window()):
		v.Add(string(PathFrom), string(CodeBeyondRetention), fmt.Sprintf("Events older than %d days are no longer retained. Pick a later time.", r.window().Days()))
	}
	return v.OrNil()
}

func (r ReplayRules) window() ReplayWindow {
	if r.Window <= 0 {
		return DefaultReplayWindow
	}
	return r.Window
}
