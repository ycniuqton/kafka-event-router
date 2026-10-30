package jobs

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/dlq"
)

// PageSource yields the pages of one kind of job (match scan, explicit ids, one edited replay).
type PageSource interface {
	Total(ctx context.Context, j Job) int
	Next(ctx context.Context, j Job, size int) (dlq.JobPage, error)
}

// PageSources picks the page source of a job from its stored selection.
type PageSources struct {
	Pages Pages
}

// For returns the edited replay, the id pages or the match pages of j.
func (s PageSources) For(j Job) PageSource {
	switch {
	case j.Edit != nil && len(j.Match.IDs) == 1:
		return EditedReplay{Pages: s.Pages}
	case len(j.Match.IDs) > 0:
		return IDPages{Pages: s.Pages}
	}
	return MatchPages{Pages: s.Pages}
}

// PageRequestOf builds page requests.
type PageRequestOf struct{}

// Of returns the page request of j.
func (PageRequestOf) Of(j Job, size int) dlq.PageRequest {
	return dlq.PageRequest{Kind: j.Action, Actor: dlq.Actor{UserID: j.RequestedBy}, JobID: j.ID, RouteID: j.RouteID, Cursor: j.Cursor, Size: size}
}

// MatchPages scans every unresolved record matching the job's filter.
type MatchPages struct {
	Pages Pages
}

// Total counts the matches (0 when the count fails: progress then grows with processed).
func (p MatchPages) Total(ctx context.Context, j Job) int {
	n, _ := p.Pages.CountMatches(ctx, j.RouteID, MatchOf{}.Query(j.Match))
	return n
}

// Next implements PageSource.
func (p MatchPages) Next(ctx context.Context, j Job, size int) (dlq.JobPage, error) {
	return p.Pages.ActPageSized(ctx, PageRequestOf{}.Of(j, size), MatchOf{}.Query(j.Match))
}

// IDPages walks the job's explicit ids (selection replays).
type IDPages struct {
	Pages Pages
}

// Total is the number of ids.
func (IDPages) Total(_ context.Context, j Job) int { return len(j.Match.IDs) }

// Next implements PageSource.
func (p IDPages) Next(ctx context.Context, j Job, size int) (dlq.JobPage, error) {
	return p.Pages.ActIDsPage(ctx, PageRequestOf{}.Of(j, size), j.Match.IDs)
}

// EditedReplay replays the one approved edited record.
type EditedReplay struct {
	Pages Pages
}

// Total is one record.
func (EditedReplay) Total(context.Context, Job) int { return 1 }

// Next implements PageSource: one page, always done.
func (p EditedReplay) Next(ctx context.Context, j Job, _ int) (dlq.JobPage, error) {
	report, err := p.Pages.ReplayApproved(ctx, dlq.Actor{UserID: j.RequestedBy}, j.ID, j.RouteID, j.Match.IDs[0], j.Edit)
	return dlq.JobPage{Report: report, Done: true}, err
}

// PacerWindow is how much work one page covers at the job's rate.
type PacerWindow time.Duration

const PageWindow PacerWindow = PacerWindow(2 * time.Second)

// Pacer keeps a job at or below Rate events per second (token bucket of one page window); Unlimited pacers
// (discard jobs: nothing is produced) run full pages back to back.
type Pacer struct {
	Rate      int
	Now       func() time.Time
	Unlimited bool
}

// PacerOf picks the pacer of a job: replays (retry) are rate-limited, discards are not.
type PacerOf struct{}

// For returns the pacer of j.
func (PacerOf) For(j Job, now func() time.Time) Pacer {
	return Pacer{Rate: j.Rate, Now: now, Unlimited: j.Action != dlq.ActionRetry}
}

// rate returns the effective rate (DefaultRate when unset or invalid).
func (p Pacer) rate() int {
	if !Rate(p.Rate).Valid() {
		return int(DefaultRate)
	}
	return p.Rate
}

// Size is the page size covering one PageWindow at the rate (1..MaxSyncAction).
func (p Pacer) Size() int {
	if p.Unlimited {
		return int(dlq.MaxSyncAction)
	}
	n := int(float64(p.rate()) * time.Duration(PageWindow).Seconds())
	return min(max(n, 1), int(dlq.MaxSyncAction))
}

// Delay returns how long to wait after n events that started at started.
func (p Pacer) Delay(n int, started time.Time) time.Duration {
	if p.Unlimited {
		return 0
	}
	due := started.Add(time.Duration(float64(n) / float64(p.rate()) * float64(time.Second)))
	return max(due.Sub(p.Now()), 0)
}

// Wait sleeps for Delay or until ctx ends.
func (p Pacer) Wait(ctx context.Context, n int, started time.Time) error {
	d := p.Delay(n, started)
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
