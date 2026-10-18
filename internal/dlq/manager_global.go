package dlq

import (
	"context"
	"sort"
	"sync"

	"github.com/eventrouter/router/internal/dlq/record"
)

// RouteScan is the scan result of one route of the global list.
type RouteScan struct {
	Route  RouteInfo
	Loc    Location
	Cursor Cursor
	Page   ScanPage
	Err    error
}

// ListGlobal merges the newest dead letters of routes (design §5.2); a failing route only warns.
func (m *Manager) ListGlobal(ctx context.Context, routes []RouteInfo, q Query) (ListPage, error) {
	gc, err := CursorCodec{}.DecodeGlobal(q.Cursor)
	if err != nil {
		return ListPage{}, err
	}
	scans := Fanout[RouteScan]{Limit: int(GlobalParallelism)}.Run(routes, func(r RouteInfo) RouteScan {
		return m.routeScan(ctx, r, gc, q)
	})
	merged, next := GlobalMerger{}.Merge(scans, q.PageSize())
	out := ListPage{Items: []ItemView{}}
	for _, s := range scans {
		out.Scanned += s.Page.Scanned
		out.Truncated = out.Truncated || s.Page.Truncated
		out.Warnings = append(out.Warnings, (WarningsOf{}).Route(s)...)
	}
	if out.Items, err = m.globalItems(ctx, merged, q.IncludeResolved); err != nil {
		return ListPage{}, err
	}
	if next != nil {
		enc := CursorCodec{}.Encode(*next)
		out.NextCursor = &enc
	}
	return out, nil
}

// globalItems renders merged rows; resolutions are looked up per route when resolved rows are included.
func (m *Manager) globalItems(ctx context.Context, merged []GlobalItem, withResolution bool) ([]ItemView, error) {
	byRoute := map[string]Resolutions{}
	if withResolution {
		for _, group := range (GlobalGroups{}).ByRoute(merged) {
			found, err := m.resolutions(ctx, group[0].Loc, (GlobalGroups{}).Records(group))
			if err != nil {
				return nil, err
			}
			byRoute[group[0].Route.ID] = found
		}
	}
	out := make([]ItemView, 0, len(merged))
	for _, it := range merged {
		out = append(out, it.Loc.Presenter().Item(it.Route, it.Record, byRoute[it.Route.ID].Of(it.Record.Ref)))
	}
	return out, nil
}

// GlobalGroups groups merged rows.
type GlobalGroups struct{}

// ByRoute groups rows by route id.
func (GlobalGroups) ByRoute(items []GlobalItem) map[string][]GlobalItem {
	out := map[string][]GlobalItem{}
	for _, it := range items {
		out[it.Route.ID] = append(out[it.Route.ID], it)
	}
	return out
}

// Records returns the records of rows.
func (GlobalGroups) Records(items []GlobalItem) []record.Record {
	out := make([]record.Record, 0, len(items))
	for _, it := range items {
		out = append(out, it.Record)
	}
	return out
}

// routeScan scans one route from its part of the global cursor.
func (m *Manager) routeScan(ctx context.Context, r RouteInfo, gc GlobalCursor, q Query) RouteScan {
	cur, ok := gc.Routes[r.ID]
	if !ok {
		cur = NewCursor()
	}
	if cur.Done {
		return RouteScan{Route: r, Cursor: cur, Page: ScanPage{Next: cur}}
	}
	loc, err := m.d.Locator.Locate(ctx, r.ID)
	if err != nil {
		return RouteScan{Route: r, Cursor: cur, Err: err}
	}
	page, err := m.scan(ctx, loc, q, cur, q.PageSize())
	return RouteScan{Route: loc.Route, Loc: loc, Cursor: cur, Page: page, Err: err}
}

// Route returns the warnings of one route scan.
func (WarningsOf) Route(s RouteScan) []RouteWarning {
	if s.Err != nil {
		return []RouteWarning{{RouteID: s.Route.ID, Reason: WarnScanFailed}}
	}
	return WarningsOf{}.Missing(s.Route.ID, s.Page.Missing)
}

// GlobalItem is one merged row with its route.
type GlobalItem struct {
	Route  RouteInfo
	Loc    Location
	Steps  StepLabels
	Record record.Record
}

// GlobalMerger merges route pages newest-first and builds the composite cursor.
type GlobalMerger struct{}

// Merge returns the newest want rows and the next cursor (nil when every route is done).
func (GlobalMerger) Merge(scans []RouteScan, want int) ([]GlobalItem, *GlobalCursor) {
	all := []GlobalItem{}
	for _, s := range scans {
		for _, r := range s.Page.Records {
			all = append(all, GlobalItem{Route: s.Route, Loc: s.Loc, Steps: s.Loc.Steps, Record: r})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return GlobalOrder{}.Before(all[i], all[j]) })
	if len(all) > want {
		all = all[:want]
	}
	taken := map[string][]record.Record{}
	for _, it := range all {
		taken[it.Route.ID] = append(taken[it.Route.ID], it.Record)
	}
	next, done := GlobalCursor{V: CurrentCursorVersion, Routes: map[string]Cursor{}}, true
	for _, s := range scans {
		c := RouteCursorOf{}.Next(s, taken[s.Route.ID])
		next.Routes[s.Route.ID] = c
		done = done && c.Done
	}
	if done {
		return all, nil
	}
	return all, &next
}

// GlobalOrder orders global rows newest first (route id, then offset break ties).
type GlobalOrder struct{}

// Before reports whether a sorts before b.
func (GlobalOrder) Before(a, b GlobalItem) bool {
	if !a.Record.Failure.FailedAt.Equal(b.Record.Failure.FailedAt) {
		return a.Record.Failure.FailedAt.After(b.Record.Failure.FailedAt)
	}
	if a.Route.ID != b.Route.ID {
		return a.Route.ID < b.Route.ID
	}
	return a.Record.Ref.Offset > b.Record.Ref.Offset
}

// RouteCursorOf computes a route's next cursor from the rows the global page took from it.
type RouteCursorOf struct{}

// Next keeps untaken rows above the bound; a failed route is done (its warning was shown).
func (RouteCursorOf) Next(s RouteScan, taken []record.Record) Cursor {
	if s.Err != nil || s.Cursor.Done {
		return Cursor{V: CurrentCursorVersion, Bounds: map[PartitionKey]int64{}, Done: true}
	}
	if len(taken) == len(s.Page.Records) {
		return s.Page.Next
	}
	c := Cursor{V: CurrentCursorVersion, Bounds: map[PartitionKey]int64{}}
	for k, v := range s.Cursor.Bounds {
		c.Bounds[k] = v
	}
	pageBy, takenBy := (RecordsBy{}).Partition(s.Page.Records), (RecordsBy{}).Partition(taken)
	for k, bound := range s.Page.Next.Bounds {
		switch {
		case len(takenBy[k]) == len(pageBy[k]):
			c.Bounds[k] = bound
		case len(takenBy[k]) > 0:
			c.Bounds[k] = (RecordsBy{}).OldestOffset(takenBy[k])
		}
	}
	return c
}

// RecordsBy groups records.
type RecordsBy struct{}

// Partition groups records by partition key.
func (RecordsBy) Partition(recs []record.Record) map[PartitionKey][]record.Record {
	out := map[PartitionKey][]record.Record{}
	for _, r := range recs {
		k := KeyOf(r.Ref.Topic, r.Ref.Partition)
		out[k] = append(out[k], r)
	}
	return out
}

// OldestOffset returns the smallest offset of recs.
func (RecordsBy) OldestOffset(recs []record.Record) int64 {
	oldest := recs[0].Ref.Offset
	for _, r := range recs {
		oldest = min(oldest, r.Ref.Offset)
	}
	return oldest
}

// Fanout runs a function over items with bounded parallelism, keeping input order.
type Fanout[T any] struct {
	Limit int
}

// Run returns fn(item) for each item.
func (f Fanout[T]) Run(items []RouteInfo, fn func(RouteInfo) T) []T {
	out := make([]T, len(items))
	sem := make(chan struct{}, max(f.Limit, 1))
	var wg sync.WaitGroup
	for i, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			out[i] = fn(it)
		}()
	}
	wg.Wait()
	return out
}
