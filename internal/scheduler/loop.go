package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/changes"
	"github.com/eventrouter/router/internal/runtime/domain"
)

// SchedulerLoop campaigns for leadership and, while leader, runs reaper → planner → rollout every tick
// and shortly after change hints (design 5.8).
type SchedulerLoop struct {
	Elector LeaderElector
	Reaper  LeaseReaper
	Planner *AssignmentPlanner
	Rollout *RolloutCoordinator
	Replay  *ReplayCoordinator
	Hints   *changes.Broker
	Timings domain.Timings
	Log     *slog.Logger
	State   *LeaderState
}

// Run loops until ctx ends; standby instances retry the campaign every tick.
func (s *SchedulerLoop) Run(ctx context.Context) {
	for ctx.Err() == nil {
		lost, err := s.Elector.Campaign(ctx)
		if err != nil {
			s.wait(ctx, s.Timings.Scheduler)
			continue
		}
		s.Log.Info("scheduler: leadership acquired")
		s.State.set(true)
		s.lead(ctx, lost)
		s.State.set(false)
		s.Log.Info("scheduler: leadership ended")
	}
}

// lead runs passes until leadership is lost.
func (s *SchedulerLoop) lead(ctx context.Context, lost <-chan struct{}) {
	hints, cancel := s.Hints.Subscribe()
	defer cancel()
	tick := time.NewTicker(s.Timings.Scheduler)
	defer tick.Stop()
	s.Pass(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-lost:
			return
		case <-tick.C:
			s.Pass(ctx)
		case <-hints:
			s.debounce(ctx, hints)
			s.Pass(ctx)
		}
	}
}

// Pass runs one reaper → replay → planner → rollout pass, logging failures; replays go first so a
// stopping replay's leases are removed by the planner in the same pass.
func (s *SchedulerLoop) Pass(ctx context.Context) {
	if n, err := s.Reaper.Reap(ctx); err != nil {
		s.Log.Warn("scheduler: reaper failed", string(logging.AttrError), err)
	} else if n > 0 {
		s.Log.Info("scheduler: expired leases removed", string(LogAttrCount), n)
	}
	if s.Replay != nil {
		if err := s.Replay.Advance(ctx); err != nil {
			s.Log.Warn("scheduler: replay failed", string(logging.AttrError), err)
		}
	}
	if err := s.Planner.Plan(ctx); err != nil {
		s.Log.Warn("scheduler: planning failed", string(logging.AttrError), err)
	}
	if err := s.Rollout.Advance(ctx); err != nil {
		s.Log.Warn("scheduler: rollout failed", string(logging.AttrError), err)
	}
}

// debounce waits briefly and drains queued hints so a burst causes one pass.
func (s *SchedulerLoop) debounce(ctx context.Context, hints <-chan domain.ChangeHint) {
	s.wait(ctx, domain.ChangeDebounce.D())
	for {
		select {
		case <-hints:
		default:
			return
		}
	}
}

func (s *SchedulerLoop) wait(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// LogAttr names scheduler log attributes.
type LogAttr string

const LogAttrCount LogAttr = "count"
