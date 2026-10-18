package reconcile

import (
	"context"
	"sync"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
)

// HaltReporter is the control-plane call that records a halt.
type HaltReporter interface {
	ReportHalt(ctx context.Context, h domain.HaltReport) error
}

// HaltQueue keeps halts until the control plane acknowledges them (design 5.3: resend on reconnect).
type HaltQueue struct {
	mu      sync.Mutex
	pending map[string]domain.HaltReport
	notify  func()
}

// NewHaltQueue builds the queue; notify is called when a halt arrives (e.g. Reconciler.Trigger).
func NewHaltQueue() *HaltQueue {
	return &HaltQueue{pending: map[string]domain.HaltReport{}, notify: func() {}}
}

// OnHalt sets the arrival callback.
func (q *HaltQueue) OnHalt(fn func()) { q.notify = fn }

// Halted implements runner.HaltSink.
func (q *HaltQueue) Halted(routeID string, ref domain.VersionRef, h *failures.HaltError) {
	q.mu.Lock()
	q.pending[routeID] = domain.HaltReport{RouteID: routeID, VersionID: ref.VersionID, Reason: h.Reason, ErrorCategory: h.Category, Message: h.Message}
	q.mu.Unlock()
	q.notify()
}

// Flush sends every pending halt; failures stay queued.
func (q *HaltQueue) Flush(ctx context.Context, to HaltReporter) {
	for _, h := range q.snapshot() {
		if to.ReportHalt(ctx, h) != nil {
			continue
		}
		q.mu.Lock()
		if cur, ok := q.pending[h.RouteID]; ok && cur == h {
			delete(q.pending, h.RouteID)
		}
		q.mu.Unlock()
	}
}

// Len returns the number of unreported halts.
func (q *HaltQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

func (q *HaltQueue) snapshot() []domain.HaltReport {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]domain.HaltReport, 0, len(q.pending))
	for _, h := range q.pending {
		out = append(out, h)
	}
	return out
}
