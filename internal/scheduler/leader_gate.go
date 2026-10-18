package scheduler

import "sync/atomic"

// LeaderGate tells leader-only jobs of other features (monitoring's alert evaluator) whether this API
// instance currently holds the scheduler leadership, so exactly one instance runs them.
type LeaderGate interface {
	IsLeader() bool
}

// LeaderState is the LeaderGate kept up to date by SchedulerLoop (a nil state is never leader).
type LeaderState struct {
	leading atomic.Bool
}

// IsLeader implements LeaderGate.
func (s *LeaderState) IsLeader() bool {
	return s != nil && s.leading.Load()
}

// set records a leadership change.
func (s *LeaderState) set(leading bool) {
	if s != nil {
		s.leading.Store(leading)
	}
}
