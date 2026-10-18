package dlq

import (
	"context"
	"fmt"
)

// JobPage is the outcome of one page of a bulk job (design §5.11).
type JobPage struct {
	Report ActionReport
	Cursor string
	Done   bool
}

// JobActions are the action kinds a bulk job may run.
var JobActions = map[ActionKind]bool{ActionRetry: true, ActionDiscard: true}

// PrepareJob validates a bulk job before it is queued: known route, job action, active version for retry.
func (m *Manager) PrepareJob(ctx context.Context, kind ActionKind, routeID string) (RouteInfo, error) {
	handler, ok := m.actions[kind]
	if !ok || !JobActions[kind] {
		return RouteInfo{}, ErrJobAction.At(string(PathAction))
	}
	route, err := m.d.Routes.Route(ctx, routeID)
	if err != nil {
		return RouteInfo{}, err
	}
	if handler.NeedsActiveVersion() && !route.HasActiveVersion() {
		return RouteInfo{}, ErrNoActiveVersion.At(string(PathActive))
	}
	return route, nil
}

// ActPage applies kind to the next page (≤ MaxSyncAction) of q below cursor; resolved records are skipped
// by the scan and the cursor only moves down, so a resumed job never acts on a page twice.
func (m *Manager) ActPage(ctx context.Context, kind ActionKind, actor Actor, jobID, routeID string, q Query, cursor string) (JobPage, error) {
	handler, ok := m.actions[kind]
	if !ok {
		return JobPage{}, fmt.Errorf("dlq: no action handler %q", kind)
	}
	loc, err := m.d.Locator.Locate(ctx, routeID)
	if err != nil {
		return JobPage{}, err
	}
	ac := ActionContext{Location: loc, Actor: actor, Now: m.d.Now(), JobID: jobID}
	if handler.NeedsActiveVersion() {
		if ac.Target, err = m.retryTarget(ctx, loc.Route); err != nil {
			return JobPage{}, err
		}
	}
	cur, err := CursorCodec{}.Decode(cursor)
	if err != nil {
		return JobPage{}, err
	}
	page, err := m.scan(ctx, loc, q.WithoutPaging(), cur, int(MaxSyncAction))
	if err != nil {
		return JobPage{}, err
	}
	report := handler.Apply(ctx, ac, page.Records)
	m.d.Summaries.Invalidate(ctx, routeID)
	m.d.Metrics.Action(report)
	out := JobPage{Report: report, Done: page.Next.Done}
	if !page.Next.Done {
		out.Cursor = CursorCodec{}.Encode(page.Next)
	}
	return out, nil
}

// CountMatches counts the unresolved records of q (bounded by MaxJobSize) for job progress totals.
func (m *Manager) CountMatches(ctx context.Context, routeID string, q Query) (int, error) {
	loc, err := m.d.Locator.Locate(ctx, routeID)
	if err != nil {
		return 0, err
	}
	cur, n := NewCursor(), 0
	for !cur.Done && n < int(MaxJobSize) {
		page, err := m.scan(ctx, loc, q.WithoutPaging(), cur, int(MaxExport))
		if err != nil {
			return 0, err
		}
		n += len(page.Records)
		cur = page.Next
	}
	return n, nil
}
