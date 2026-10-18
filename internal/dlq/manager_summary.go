package dlq

import (
	"context"

	"github.com/eventrouter/router/internal/dlq/record"
)

// Summary returns the unresolved counts of a route (design §5.3), cached for SummaryTTL.
func (m *Manager) Summary(ctx context.Context, routeID string) (Summary, error) {
	if s, ok := m.d.Summaries.Get(ctx, routeID); ok {
		return s, nil
	}
	loc, err := m.d.Locator.Locate(ctx, routeID)
	if err != nil {
		return Summary{}, err
	}
	total, err := m.watermarkTotal(ctx, loc)
	if err != nil {
		return Summary{}, err
	}
	page, err := m.scan(ctx, loc, Query{}, NewCursor(), int(SummarySample))
	if err != nil {
		return Summary{}, err
	}
	s := (SummaryOf{}).Build(page, total)
	m.d.Summaries.Set(ctx, routeID, s)
	return s, nil
}

// watermarkTotal returns Σ(high − low) minus resolved entries still in retention.
func (m *Manager) watermarkTotal(ctx context.Context, loc Location) (int, error) {
	total := 0
	for _, t := range loc.Targets {
		set, err := m.d.Reader.Ranges(ctx, []Target{t})
		if err != nil {
			return 0, err
		}
		for _, r := range set.Ranges {
			total += int(r.High - r.Low)
		}
		resolved, err := m.d.State.CountResolved(ctx, t.ConnectionID, loc.Route.ID, set.Ranges)
		if err != nil {
			return 0, err
		}
		total -= resolved
	}
	return max(total, 0), nil
}

// SummaryOf builds summaries from a sample page.
type SummaryOf struct{}

// Build is exact when the sample covered the whole DLQ, else approximate from watermarks.
func (SummaryOf) Build(page ScanPage, total int) Summary {
	s := Summary{ByErrorType: map[record.ErrorType]int{}, Warnings: nil}
	for _, r := range page.Records {
		s.ByErrorType[r.Failure.ErrorType]++
		at := r.Failure.FailedAt
		if s.NewestAt == nil || at.After(*s.NewestAt) {
			s.NewestAt = &at
		}
		if s.OldestAt == nil || at.Before(*s.OldestAt) {
			s.OldestAt = &at
		}
	}
	exact := page.Next.Done && !page.Truncated
	s.Unresolved, s.Approximate = len(page.Records), !exact
	if !exact {
		s.Unresolved = max(total, len(page.Records))
	}
	if len(page.Missing) > 0 {
		s.Warnings = []Warning{WarnTopicMissing}
	}
	return s
}

// GlobalSummary is the badge count over routes, with per-type counts for the global filter chips.
type GlobalSummary struct {
	Unresolved  int                      `json:"unresolved"`
	ByErrorType map[record.ErrorType]int `json:"byErrorType"`
	Approximate bool                     `json:"approximate"`
	Routes      []RouteSummary           `json:"routes"`
}

// RouteSummary is one route's unresolved count.
type RouteSummary struct {
	RouteID     string                   `json:"routeId"`
	RouteName   string                   `json:"routeName"`
	Unresolved  int                      `json:"unresolved"`
	ByErrorType map[record.ErrorType]int `json:"byErrorType"`
	Approximate bool                     `json:"approximate"`
}

// GlobalSummary sums the summaries of routes; failing or empty routes are left out.
func (m *Manager) GlobalSummary(ctx context.Context, routes []RouteInfo) GlobalSummary {
	sums := Fanout[*RouteSummary]{Limit: int(GlobalParallelism)}.Run(routes, func(r RouteInfo) *RouteSummary {
		if !r.HasActiveVersion() {
			return nil
		}
		s, err := m.Summary(ctx, r.ID)
		if err != nil || s.Unresolved == 0 {
			return nil
		}
		return &RouteSummary{RouteID: r.ID, RouteName: r.Name, Unresolved: s.Unresolved, ByErrorType: s.ByErrorType, Approximate: s.Approximate}
	})
	out := GlobalSummary{ByErrorType: map[record.ErrorType]int{}, Routes: []RouteSummary{}}
	for _, s := range sums {
		if s != nil {
			out.add(*s)
		}
	}
	return out
}

func (g *GlobalSummary) add(s RouteSummary) {
	g.Unresolved += s.Unresolved
	g.Approximate = g.Approximate || s.Approximate
	for t, n := range s.ByErrorType {
		g.ByErrorType[t] += n
	}
	g.Routes = append(g.Routes, s)
}
