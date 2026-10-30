package dlq

import (
	"context"
	"fmt"
	"strconv"
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
	return m.ActPageSized(ctx, PageRequest{Kind: kind, Actor: actor, JobID: jobID, RouteID: routeID, Cursor: cursor, Size: int(MaxSyncAction)}, q)
}

// PageRequest is one page of a bulk job: who acts, where to resume and how many records (rate limiting).
type PageRequest struct {
	Kind    ActionKind
	Actor   Actor
	JobID   string
	RouteID string
	Cursor  string
	Size    int
}

// jobContext resolves the handler, location and (for retries) active version of a job page.
func (m *Manager) jobContext(ctx context.Context, req PageRequest) (ActionHandler, ActionContext, error) {
	handler, ok := m.actions[req.Kind]
	if !ok {
		return nil, ActionContext{}, fmt.Errorf("dlq: no action handler %q", req.Kind)
	}
	loc, err := m.d.Locator.Locate(ctx, req.RouteID)
	if err != nil {
		return nil, ActionContext{}, err
	}
	ac := ActionContext{Location: loc, Actor: req.Actor, Now: m.d.Now(), JobID: req.JobID}
	if handler.NeedsActiveVersion() {
		if ac.Target, err = m.retryTarget(ctx, loc.Route); err != nil {
			return nil, ActionContext{}, err
		}
	}
	return handler, ac, nil
}

// ActPageSized is ActPage with an explicit page size (1..MaxSyncAction).
func (m *Manager) ActPageSized(ctx context.Context, req PageRequest, q Query) (JobPage, error) {
	handler, ac, err := m.jobContext(ctx, req)
	if err != nil {
		return JobPage{}, err
	}
	cur, err := CursorCodec{}.Decode(req.Cursor)
	if err != nil {
		return JobPage{}, err
	}
	page, err := m.scan(ctx, ac.Location, q.WithoutPaging(), cur, (PageSize{}).Clamp(req.Size))
	if err != nil {
		return JobPage{}, err
	}
	report := handler.Apply(ctx, ac, page.Records)
	m.d.Summaries.Invalidate(ctx, req.RouteID)
	m.d.Metrics.Action(report)
	out := JobPage{Report: report, Done: page.Next.Done}
	if !page.Next.Done {
		out.Cursor = CursorCodec{}.Encode(page.Next)
	}
	return out, nil
}

// ActIDsPage applies the job action to the next Size ids after cursor (an index into ids).
func (m *Manager) ActIDsPage(ctx context.Context, req PageRequest, ids []string) (JobPage, error) {
	handler, ac, err := m.jobContext(ctx, req)
	if err != nil {
		return JobPage{}, err
	}
	start, _ := strconv.Atoi(req.Cursor)
	end := min(len(ids), max(start, 0)+(PageSize{}).Clamp(req.Size))
	if start >= end {
		return JobPage{Done: true}, nil
	}
	resolved, err := m.selector.Resolve(ctx, ac.Location, (SelectionOf{}).For(handler, Selection{IDs: ids[start:end]}), ActionCap)
	if err != nil {
		return JobPage{}, err
	}
	report := handler.Apply(ctx, ac, resolved.Records)
	report.Results = append(resolved.Skipped, report.Results...)
	m.d.Summaries.Invalidate(ctx, req.RouteID)
	m.d.Metrics.Action(report)
	return JobPage{Report: report, Cursor: strconv.Itoa(end), Done: end >= len(ids)}, nil
}

// PageSize clamps job page sizes.
type PageSize struct{}

// Clamp returns n within [1, MaxSyncAction] (0 = MaxSyncAction).
func (PageSize) Clamp(n int) int {
	if n <= 0 || n > int(MaxSyncAction) {
		return int(MaxSyncAction)
	}
	return n
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
