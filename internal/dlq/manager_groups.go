package dlq

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// GroupCache keeps group pages for GroupCacheTTL (grouping scans up to MaxGroupScan records).
type GroupCache interface {
	Get(key string) (GroupPage, bool)
	Set(key string, page GroupPage)
}

// MemoryGroupCache is the per-instance group cache.
type MemoryGroupCache struct {
	mu      sync.Mutex
	entries map[string]groupEntry
	now     func() time.Time
}

type groupEntry struct {
	page GroupPage
	at   time.Time
}

// NewMemoryGroupCache returns an empty cache.
func NewMemoryGroupCache(now func() time.Time) *MemoryGroupCache {
	return &MemoryGroupCache{entries: map[string]groupEntry{}, now: now}
}

// Get implements GroupCache.
func (c *MemoryGroupCache) Get(key string) (GroupPage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || c.now().Sub(e.at) > GroupCacheTTL.Duration() {
		return GroupPage{}, false
	}
	return e.page, true
}

// Set implements GroupCache; expired entries are dropped on write.
func (c *MemoryGroupCache) Set(key string, page GroupPage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		if c.now().Sub(e.at) > GroupCacheTTL.Duration() {
			delete(c.entries, k)
		}
	}
	c.entries[key] = groupEntry{page: page, at: c.now()}
}

// GroupCacheKey is the cache key of a route's grouping query.
type GroupCacheKey struct{}

// Of returns the key of routeID and q (filters only).
func (GroupCacheKey) Of(routeID string, q Query) string {
	return fmt.Sprintf("%s|%v|%v|%d|%d|%s|%s", routeID, q.ErrorTypes, q.Classes, q.Since.Unix(), q.Until.Unix(), q.Step, q.Text)
}

// RouteGroups is the grouping of one route.
type RouteGroups struct {
	Route   RouteInfo
	Set     *GroupSet
	Scanned int
	Partial bool
	Trunc   bool
	Missing []string
	Err     error
}

// Groups returns the cause groups of a route's unresolved dead letters (design 00001 §3.2).
func (m *Manager) Groups(ctx context.Context, routeID string, q Query) (GroupPage, error) {
	q = q.WithoutPaging()
	key := GroupCacheKey{}.Of(routeID, q)
	if page, ok := m.groupCache().Get(key); ok {
		return page, nil
	}
	rg := m.routeGroups(ctx, RouteInfo{ID: routeID}, q, TrendSpecOf{}.For(q, m.d.Now()), int(MaxGroupScan))
	if rg.Err != nil {
		return GroupPage{}, rg.Err
	}
	page := rg.Set.PageOf(rg.Scanned, rg.Trunc, rg.Partial)
	page.Warnings = WarningsOf{}.Missing("", rg.Missing)
	m.groupCache().Set(key, page)
	return page, nil
}

// GlobalGroups groups the dead letters of every given route; a failing route only warns.
func (m *Manager) GlobalGroups(ctx context.Context, routes []RouteInfo, q Query) GroupPage {
	q = q.WithoutPaging()
	spec := TrendSpecOf{}.For(q, m.d.Now())
	parts := Fanout[RouteGroups]{Limit: int(GlobalParallelism)}.Run(routes, func(r RouteInfo) RouteGroups {
		return m.routeGroups(ctx, r, q, spec, int(MaxGroupScanPerRoute))
	})
	all, scanned, trunc, partial, warnings := NewGroupSet(spec), 0, false, false, []RouteWarning{}
	for _, p := range parts {
		if p.Err != nil {
			warnings = append(warnings, RouteWarning{RouteID: p.Route.ID, Reason: WarnScanFailed})
			continue
		}
		all.Merge(p.Set)
		scanned, trunc, partial = scanned+p.Scanned, trunc || p.Trunc, partial || p.Partial
		warnings = append(warnings, WarningsOf{}.Missing(p.Route.ID, p.Missing)...)
	}
	page := all.PageOf(scanned, trunc, partial)
	page.Warnings = warnings
	return page
}

// routeGroups scans up to limit unresolved records of a route and groups them.
func (m *Manager) routeGroups(ctx context.Context, r RouteInfo, q Query, spec TrendSpec, limit int) RouteGroups {
	loc, err := m.d.Locator.Locate(ctx, r.ID)
	if err != nil {
		return RouteGroups{Route: r, Err: err}
	}
	page, err := m.scan(ctx, loc, q, NewCursor(), limit)
	if err != nil {
		return RouteGroups{Route: loc.Route, Err: err}
	}
	set := NewGroupSet(spec)
	set.Add(loc, page.Records)
	return RouteGroups{Route: loc.Route, Set: set, Scanned: page.Scanned, Trunc: page.Truncated, Partial: !page.Next.Done, Missing: page.Missing}
}

// groupCache returns the configured cache (none configured: a fresh memory cache per Manager).
func (m *Manager) groupCache() GroupCache {
	m.groupsOnce.Do(func() {
		if m.d.Groups == nil {
			m.d.Groups = NewMemoryGroupCache(m.d.Now)
		}
	})
	return m.d.Groups
}
