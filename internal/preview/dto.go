package preview

import (
	"time"

	"github.com/eventrouter/router/pkg/config"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/trace"
	"github.com/eventrouter/router/pkg/kafka"
)

// SessionDTO is the public view of a session.
type SessionDTO struct {
	ID         SessionID         `json:"id"`
	ExpiresAt  time.Time         `json:"expiresAt"`
	Loaded     int               `json:"loaded"`
	Bytes      int64             `json:"bytes"`
	Truncated  bool              `json:"truncated"`
	StopReason kafka.StopReason  `json:"stopReason"`
	Masked     bool              `json:"masked"`
	Format     config.FormatType `json:"format"`
}

// RawDTO is the exact record bytes rendered as text.
type RawDTO struct {
	Key      *string     `json:"key"`
	Value    string      `json:"value"`
	Encoding RawEncoding `json:"encoding"`
}

// SampleDTO is one sample as shown by the panel.
type SampleDTO struct {
	Seq         int                `json:"seq"`
	Topic       string             `json:"topic"`
	Partition   int32              `json:"partition"`
	Offset      int64              `json:"offset"`
	Timestamp   time.Time          `json:"timestamp"`
	Key         any                `json:"key"`
	Value       any                `json:"value"`
	Headers     []engine.HeaderDoc `json:"headers"`
	Raw         RawDTO             `json:"raw"`
	DecodeError *string            `json:"decodeError"`
}

// LoadResponseDTO answers POST /preview/samples.
type LoadResponseDTO struct {
	Session    SessionDTO  `json:"session"`
	Events     []SampleDTO `json:"events"`
	NextCursor Cursor      `json:"nextCursor,omitempty"`
}

// FormatRequest is the body of PATCH /preview/sessions/{id} (per-session format override).
type FormatRequest struct {
	Format config.FormatType `json:"format"`
}

// SamplesPageDTO answers GET /preview/sessions/{id}/events.
type SamplesPageDTO struct {
	Events     []SampleDTO `json:"events"`
	NextCursor Cursor      `json:"nextCursor,omitempty"`
}

// RunRequest is the body of POST /preview/run and /preview/test-runs; WriteToTopic (test runs only)
// also produces passed outputs to a temporary test topic. SampleSessionID names the session inline
// events were copied from (Transform Studio): masking then follows that session's source.
type RunRequest struct {
	SessionID       SessionID     `json:"sessionId,omitempty"`
	Events          []InlineEvent `json:"events,omitempty"`
	SampleSessionID SessionID     `json:"sampleSessionId,omitempty"`
	Config          RawConfig     `json:"config"`
	EventSeqs       []int         `json:"eventSeqs,omitempty"`
	WriteToTopic    bool          `json:"writeToTopic,omitempty"`
	RouteID         string        `json:"routeId,omitempty"`
}

// MaskingSession is the session whose source decides the run's masking ("" = none: the draft source decides).
func (r RunRequest) MaskingSession() SessionID {
	if r.SessionID != "" {
		return r.SessionID
	}
	return r.SampleSessionID
}

// SessionSourceDTO is where a session's samples come from (GET /internal/v1/preview/sessions/{id});
// the gateway decides run masking from it. ConnectionID is empty for uploads and connection-less demo data.
type SessionSourceDTO struct {
	ID           SessionID  `json:"id"`
	ConnectionID string     `json:"connectionId,omitempty"`
	Mode         SampleMode `json:"mode"`
	Masked       bool       `json:"masked"`
}

// RawConfig is a route config draft kept as raw JSON.
type RawConfig []byte

// MarshalJSON writes the raw document (null when empty).
func (c RawConfig) MarshalJSON() ([]byte, error) {
	if len(c) == 0 {
		return []byte(JSONNull), nil
	}
	return c, nil
}

// UnmarshalJSON keeps a copy of the raw document.
func (c *RawConfig) UnmarshalJSON(b []byte) error {
	*c = append((*c)[:0], b...)
	return nil
}

// JSONLiteral names JSON literals written by hand.
type JSONLiteral string

const JSONNull JSONLiteral = "null"

// SummaryDTO counts the outcomes of a run.
type SummaryDTO struct {
	Loaded     int                `json:"loaded"`
	Processed  int                `json:"processed"`
	Passed     int                `json:"passed"`
	Filtered   int                `json:"filtered"`
	Errors     int                `json:"errors"`
	Dropped    int                `json:"dropped"`
	Incomplete bool               `json:"incomplete"`
	DurationMs int64              `json:"durationMs"`
	Branches   []BranchSummaryDTO `json:"branches"`
}

// BranchSummaryDTO counts the outcomes of one branch over the processed events (FR-14).
type BranchSummaryDTO struct {
	Branch   string `json:"branch"`
	Passed   int    `json:"passed"`
	Filtered int    `json:"filtered"`
	Errors   int    `json:"errors"`
	Dropped  int    `json:"dropped"`
}

// IssueDTO is one compile/validation finding or a coercion warning.
type IssueDTO struct {
	Path     string `json:"path"`
	Severity string `json:"severity"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message"`
}

// FilterNodeDTO is one evaluated filter node addressed by its absolute JSON Pointer.
type FilterNodeDTO struct {
	Path    string `json:"path"`
	Result  bool   `json:"result"`
	Warning string `json:"warning,omitempty"`
}

// StepTraceDTO is one executed step; Patch is relative to the previous step (or the input).
type StepTraceDTO struct {
	Index      int             `json:"index"`
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Patch      []trace.PatchOp `json:"patch"`
	DurationUs int64           `json:"durationUs"`
	Logs       []string        `json:"logs"`
	Error      *string         `json:"error"`
}

// ErrorDTO describes what ended an event or branch.
type ErrorDTO struct {
	StepIndex *int   `json:"stepIndex"`
	Step      string `json:"step,omitempty"`
	Path      string `json:"path,omitempty"`
	ErrorType string `json:"errorType"`
	Message   string `json:"message"`
}

// OutputDTO is one encoded output record.
type OutputDTO struct {
	Topic           string             `json:"topic"`
	Key             any                `json:"key"`
	Value           any                `json:"value"`
	Headers         []engine.HeaderDoc `json:"headers"`
	SizeBytes       int                `json:"sizeBytes"`
	Valid           bool               `json:"valid"`
	ValidationError *string            `json:"validationError"`
}

// BranchResultDTO is the result of one branch for one event.
type BranchResultDTO struct {
	Branch  string          `json:"branch"`
	Status  EventStatus     `json:"status"`
	Filter  []FilterNodeDTO `json:"filter"`
	Steps   []StepTraceDTO  `json:"steps"`
	Error   *ErrorDTO       `json:"error"`
	Outputs []OutputDTO     `json:"outputs"`
}

// EventResultDTO is the result of one event.
type EventResultDTO struct {
	Seq      int               `json:"seq"`
	Status   EventStatus       `json:"status"`
	Error    *ErrorDTO         `json:"error"`
	Branches []BranchResultDTO `json:"branches"`
}

// RunResultDTO answers POST /preview/run.
type RunResultDTO struct {
	Summary   SummaryDTO       `json:"summary"`
	Issues    []IssueDTO       `json:"issues"`
	Results   []EventResultDTO `json:"results"`
	Masked    bool             `json:"masked"`
	TestTopic *TestTopicDTO    `json:"testTopic,omitempty"`
}

// TestTopicDTO describes the temporary topic a test run wrote its passed outputs to.
type TestTopicDTO struct {
	ConnectionID string    `json:"connectionId"`
	Topic        string    `json:"topic"`
	Produced     int       `json:"produced"`
	ExpiresAt    time.Time `json:"expiresAt"`
}
