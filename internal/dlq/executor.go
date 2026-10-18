package dlq

import (
	"context"
	"fmt"

	"github.com/eventrouter/router/internal/dlq/record"
)

// Outcome is the per-event result of an action.
type Outcome string

const (
	OutcomeSent      Outcome = "sent"
	OutcomeFiltered  Outcome = "filtered"
	OutcomeFailed    Outcome = "failed"
	OutcomeSkipped   Outcome = "skipped"
	OutcomeDiscarded Outcome = "discarded"
	OutcomeRestored  Outcome = "restored"
)

// Outcomes lists outcomes in report order.
var Outcomes = []Outcome{OutcomeSent, OutcomeFiltered, OutcomeFailed, OutcomeSkipped}

// SkipReason tells why an event was not acted on.
type SkipReason string

const (
	SkipLocked          SkipReason = "locked"
	SkipAlreadyResolved SkipReason = "already_resolved"
	SkipExpired         SkipReason = "expired"
	SkipNotFound        SkipReason = "not_found"
	SkipNotDiscarded    SkipReason = "not_discarded"
)

// RetryFailureKind is the error type of a failed retry.
type RetryFailureKind string

const (
	FailStep          RetryFailureKind = "step_error"
	FailTimeout       RetryFailureKind = "timeout"
	FailOutputInvalid RetryFailureKind = "output_invalid"
	FailDecode        RetryFailureKind = "decode_error"
	FailCompile       RetryFailureKind = "compile"
	FailDestination   RetryFailureKind = "destination"
)

// FailureKindOf maps a DLQ error type to the retry failure kind.
var FailureKindOf = map[record.ErrorType]RetryFailureKind{
	record.ErrorStep: FailStep, record.ErrorTimeout: FailTimeout, record.ErrorOutputInvalid: FailOutputInvalid, record.ErrorDecode: FailDecode,
}

// ItemResult is the outcome of one event.
type ItemResult struct {
	Ref         Ref
	Outcome     Outcome
	Destination *Destination
	ErrorType   RetryFailureKind
	Message     string
	Reason      SkipReason
}

// RetryTarget is the active version a retry runs through.
type RetryTarget struct {
	Route   RouteInfo
	Version VersionConfig
}

// RetryExecutor re-runs records through a version and delivers the outputs.
type RetryExecutor interface {
	Kind() ExecutorKind
	Execute(ctx context.Context, target RetryTarget, recs []record.Record) []ItemResult
}

// ExecutorKind selects the executor (ROUTER_DLQ_RETRY_EXECUTOR).
type ExecutorKind string

const (
	ExecutorInProcess ExecutorKind = "in_process"
	ExecutorPreview   ExecutorKind = "preview"
)

// ExecutorRegistry selects the configured executor.
type ExecutorRegistry struct {
	executors map[ExecutorKind]RetryExecutor
}

// NewExecutorRegistry registers the given executors.
func NewExecutorRegistry(es ...RetryExecutor) *ExecutorRegistry {
	r := &ExecutorRegistry{executors: map[ExecutorKind]RetryExecutor{}}
	for _, e := range es {
		r.executors[e.Kind()] = e
	}
	return r
}

// Build returns the executor of kind.
func (r *ExecutorRegistry) Build(kind ExecutorKind) (RetryExecutor, error) {
	e, ok := r.executors[kind]
	if !ok {
		return nil, fmt.Errorf("dlq: no retry executor %q", kind)
	}
	return e, nil
}
