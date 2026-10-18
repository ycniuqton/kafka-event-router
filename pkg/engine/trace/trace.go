// Package trace records per-event diagnostics for preview: filter node results,
// per-step RFC 6902 patches, logs, timings and the final status. NoopTracer keeps
// the production path free of snapshot cost.
package trace

import "time"

// EventStatus is the final outcome of one event.
type EventStatus string

const (
	StatusPassed   EventStatus = "passed"
	StatusFiltered EventStatus = "filtered"
	StatusError    EventStatus = "error"
	StatusDropped  EventStatus = "dropped"
)

// StepRecord is one step execution; Before/After are JSON-shaped event snapshots.
type StepRecord struct {
	Branch   string
	StepID   string
	Kind     string
	Before   []any
	After    []any
	Duration time.Duration
	Logs     []string
	Err      error
}

// ErrorInfo describes the failure that ended an event.
type ErrorInfo struct {
	Type    string `json:"type"`
	Step    string `json:"step,omitempty"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// Tracer receives diagnostics; when Enabled is false callers skip snapshot work.
type Tracer interface {
	Enabled() bool
	Node(branch, nodeID string, ok bool, warning string)
	Step(r StepRecord)
	Outputs(branch string, n int)
	Finish(status EventStatus, err *ErrorInfo)
}

// NoopTracer discards everything (production).
type NoopTracer struct{}

func (NoopTracer) Enabled() bool                     { return false }
func (NoopTracer) Node(string, string, bool, string) {}
func (NoopTracer) Step(StepRecord)                   {}
func (NoopTracer) Outputs(string, int)               {}
func (NoopTracer) Finish(EventStatus, *ErrorInfo)    {}

// OrNoop returns t, or NoopTracer when t is nil.
func OrNoop(t Tracer) Tracer {
	if t == nil {
		return NoopTracer{}
	}
	return t
}
