package failures

import (
	"context"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/config"
)

// OutcomeFunc receives how a failed record finished; a *HaltError halts the route.
type OutcomeFunc func(outcome domain.RecordOutcome, err error)

// ErrorHandler is the base of every errorPolicy.onError action.
type ErrorHandler interface {
	Action() config.ErrorAction
	Handle(ctx context.Context, f Failure, done OutcomeFunc)
}

// DLQHandler writes the original record with router.* headers to the DLQ topic.
type DLQHandler struct {
	Writer DLQWriter
}

func (DLQHandler) Action() config.ErrorAction { return config.ErrorActionDLQ }

// Handle completes once the DLQ write is acked (failed writes are retried by the writer).
func (h DLQHandler) Handle(ctx context.Context, f Failure, done OutcomeFunc) {
	h.Writer.Write(ctx, f, func(err error) {
		if err != nil {
			done("", err)
			return
		}
		done(domain.OutcomeDeadLettered, nil)
	})
}

// SkipHandler drops the record and counts it.
type SkipHandler struct{}

func (SkipHandler) Action() config.ErrorAction { return config.ErrorActionSkip }

func (SkipHandler) Handle(_ context.Context, _ Failure, done OutcomeFunc) {
	done(domain.OutcomeSkipped, nil)
}

// PauseHandler halts the route; the failed record is not marked done, so it is reprocessed on resume.
type PauseHandler struct{}

func (PauseHandler) Action() config.ErrorAction { return config.ErrorActionPause }

func (PauseHandler) Handle(_ context.Context, f Failure, done OutcomeFunc) {
	done("", NewHalt(domain.HaltErrorPolicyPause, f))
}

// RetryLimit is the default errorPolicy.maxRetries for step timeouts.
type RetryLimit int

const DefaultMaxRetries RetryLimit = 3

// ErrorPolicyManager resolves the handler of a route's errorPolicy (absent = dlq).
type ErrorPolicyManager struct {
	handlers map[config.ErrorAction]ErrorHandler
}

// NewErrorPolicyManager registers handlers by action.
func NewErrorPolicyManager(handlers ...ErrorHandler) *ErrorPolicyManager {
	m := &ErrorPolicyManager{handlers: map[config.ErrorAction]ErrorHandler{}}
	for _, h := range handlers {
		m.handlers[h.Action()] = h
	}
	return m
}

// NewDefaultPolicyManager registers dlq, skip and pause with the runner's DLQ writer.
func NewDefaultPolicyManager(writer DLQWriter) *ErrorPolicyManager {
	return NewErrorPolicyManager(DLQHandler{Writer: writer}, SkipHandler{}, PauseHandler{})
}

// Resolve returns the handler of policy; unknown actions fall back to dlq.
func (m *ErrorPolicyManager) Resolve(policy *config.ErrorPolicy) ErrorHandler {
	action := config.ErrorActionDLQ
	if policy != nil && policy.OnError != "" {
		action = policy.OnError
	}
	if h, ok := m.handlers[action]; ok {
		return h
	}
	return m.handlers[config.ErrorActionDLQ]
}

// MaxRetries returns errorPolicy.maxRetries or the default.
func (m *ErrorPolicyManager) MaxRetries(policy *config.ErrorPolicy) int {
	if policy != nil && policy.MaxRetries != nil {
		return *policy.MaxRetries
	}
	return int(DefaultMaxRetries)
}

// UsesDLQ reports whether the policy writes dead letters.
func (m *ErrorPolicyManager) UsesDLQ(policy *config.ErrorPolicy) bool {
	return m.Resolve(policy).Action() == config.ErrorActionDLQ
}
