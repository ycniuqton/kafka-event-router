package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/eventrouter/router/internal/dlq"
)

// Pages is the dlq.Manager surface a job needs.
type Pages interface {
	ActPage(ctx context.Context, kind dlq.ActionKind, actor dlq.Actor, jobID, routeID string, q dlq.Query, cursor string) (dlq.JobPage, error)
	CountMatches(ctx context.Context, routeID string, q dlq.Query) (int, error)
}

// Runner claims queued jobs and applies them page by page (every API instance runs one).
type Runner struct {
	Repo     Repository
	Pages    Pages
	Notifier Notifier
	Log      *slog.Logger
	Now      func() time.Time
}

// Run ticks until ctx ends; the reaper requeues jobs of crashed runners.
func (r *Runner) Run(ctx context.Context) {
	tick, reap := time.NewTicker(TickInterval.Duration()), time.NewTicker(ReapInterval.Duration())
	defer tick.Stop()
	defer reap.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			for r.Tick(ctx) {
			}
		case <-reap.C:
			r.Reap(ctx)
		}
	}
}

// Tick claims and runs one job; it reports whether a job ran.
func (r *Runner) Tick(ctx context.Context) bool {
	j, ok, err := r.Repo.Claim(ctx)
	if err != nil {
		r.Log.Error(string(LogRunnerError), string(LogAttrError), err)
		return false
	}
	if !ok {
		return false
	}
	r.Execute(ctx, j)
	return true
}

// Reap requeues running jobs without a heartbeat for StaleAfter.
func (r *Runner) Reap(ctx context.Context) {
	n, err := r.Repo.Requeue(ctx, r.Now().Add(-StaleAfter.Duration()))
	if err != nil {
		r.Log.Error(string(LogRunnerError), string(LogAttrError), err)
		return
	}
	if n > 0 {
		r.Log.Info(string(LogRequeued), string(LogAttrCount), n)
	}
}

// Execute runs a claimed job from its stored cursor until done, cancelled or failed.
func (r *Runner) Execute(ctx context.Context, j Job) {
	stop := Heartbeat{Repo: r.Repo}.Start(ctx, j.ID)
	defer stop()
	q := MatchOf{}.Query(j.Match)
	if j.Total == 0 && j.Cursor == "" {
		if total, err := r.Pages.CountMatches(ctx, j.RouteID, q); err == nil {
			j.Total = total
		}
	}
	for {
		page, err := r.Pages.ActPage(ctx, j.Action, dlq.Actor{UserID: j.RequestedBy}, j.ID, j.RouteID, q, j.Cursor)
		if err != nil {
			r.finish(ctx, j, StatusFailed, err.Error())
			return
		}
		j.Apply(page)
		st, err := r.Repo.Progress(ctx, j)
		switch {
		case err != nil:
			r.Log.Error(string(LogRunnerError), string(LogAttrJob), j.ID, string(LogAttrError), err)
			return
		case st == StatusCancelRequested:
			r.finish(ctx, j, StatusCancelled, "")
			return
		case page.Done:
			r.finish(ctx, j, StatusSucceeded, "")
			return
		}
	}
}

// finish stores the terminal status and notifies the requester.
func (r *Runner) finish(ctx context.Context, j Job, st Status, errText string) {
	done, err := r.Repo.Finish(ctx, j.ID, st, errText)
	if err != nil {
		r.Log.Error(string(LogRunnerError), string(LogAttrJob), j.ID, string(LogAttrError), err)
		return
	}
	r.Notifier.JobEnded(ctx, done)
}

// Heartbeat keeps a running job's heartbeat fresh while a long page runs.
type Heartbeat struct {
	Repo Repository
}

// Start beats every HeartbeatInterval until the returned stop is called.
func (h Heartbeat) Start(ctx context.Context, id string) func() {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(HeartbeatInterval.Duration())
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = h.Repo.Heartbeat(ctx, id)
			}
		}
	}()
	return cancel
}
