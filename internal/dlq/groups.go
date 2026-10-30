package dlq

import (
	"sort"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
)

// GroupLimit bounds cause grouping (design 00001 §3.2).
type GroupLimit int

const (
	MaxGroupScan         GroupLimit = 10000
	MaxGroupScanPerRoute GroupLimit = 2000
	MaxGroups            GroupLimit = 200
	TrendBucketCount     GroupLimit = 24
)

// GroupTiming holds the grouping time constants.
type GroupTiming time.Duration

const (
	DefaultTrendWindow GroupTiming = GroupTiming(24 * time.Hour)
	GroupCacheTTL      GroupTiming = GroupTiming(15 * time.Second)
)

// Duration returns the timing as a time.Duration.
func (t GroupTiming) Duration() time.Duration { return time.Duration(t) }

// TrendSpec is the bucket layout of group trends: Buckets equal buckets from Since.
type TrendSpec struct {
	Since   time.Time
	Bucket  time.Duration
	Buckets int
}

// TrendSpecOf derives the trend layout of a query: its time range, else the last 24 h before now.
type TrendSpecOf struct{}

// For returns TrendBucketCount buckets over [since, until).
func (TrendSpecOf) For(q Query, now time.Time) TrendSpec {
	until, since := now, now.Add(-DefaultTrendWindow.Duration())
	if !q.Until.IsZero() {
		until = q.Until
	}
	if !q.Since.IsZero() {
		since = q.Since
	}
	bucket := until.Sub(since) / time.Duration(TrendBucketCount)
	if bucket <= 0 {
		bucket = time.Second
	}
	return TrendSpec{Since: since, Bucket: bucket, Buckets: int(TrendBucketCount)}
}

// Index returns the bucket of at, or false outside the window.
func (s TrendSpec) Index(at time.Time) (int, bool) {
	if at.Before(s.Since) {
		return 0, false
	}
	i := int(at.Sub(s.Since) / s.Bucket)
	return i, i < s.Buckets
}

// Trend is the per-bucket count of one group.
type Trend struct {
	Since         time.Time `json:"since"`
	BucketSeconds int64     `json:"bucketSeconds"`
	Counts        []int     `json:"counts"`
}

// GroupSample is the newest record of a group.
type GroupSample struct {
	ID       string    `json:"id"`
	Message  string    `json:"message"`
	FailedAt time.Time `json:"failedAt"`
}

// CauseGroup is every dead letter sharing one fingerprint.
type CauseGroup struct {
	Fingerprint string            `json:"fingerprint"`
	RouteID     string            `json:"routeId"`
	RouteName   string            `json:"routeName"`
	Step        string            `json:"step"`
	StepLabel   string            `json:"stepLabel"`
	ErrorType   record.ErrorType  `json:"errorType"`
	ErrorClass  record.ErrorClass `json:"errorClass"`
	Pattern     string            `json:"pattern"`
	Count       int               `json:"count"`
	FirstSeenAt time.Time         `json:"firstSeenAt"`
	LastSeenAt  time.Time         `json:"lastSeenAt"`
	Versions    []int             `json:"versions"`
	Sample      GroupSample       `json:"sample"`
	Trend       Trend             `json:"trend"`
}

// observe adds one record of the group.
func (g *CauseGroup) observe(r record.Record, spec TrendSpec) {
	at := r.Failure.FailedAt
	g.Count++
	if g.FirstSeenAt.IsZero() || at.Before(g.FirstSeenAt) {
		g.FirstSeenAt = at
	}
	if !at.Before(g.LastSeenAt) {
		g.LastSeenAt = at
		g.Sample = GroupSample{ID: RefCodec{}.Encode(r.Ref), Message: r.Failure.Message, FailedAt: at}
	}
	if !(VersionList{}).Has(g.Versions, r.Failure.VersionNo) {
		g.Versions = append(g.Versions, r.Failure.VersionNo)
		sort.Ints(g.Versions)
	}
	if i, ok := spec.Index(at); ok {
		g.Trend.Counts[i]++
	}
}

// VersionList tests version lists.
type VersionList struct{}

// Has reports whether v is in list.
func (VersionList) Has(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// GroupSet aggregates scanned records into cause groups.
type GroupSet struct {
	spec    TrendSpec
	fp      Fingerprinter
	groups  map[string]*CauseGroup
	byClass ClassCounts
}

// NewGroupSet starts an empty set with the trend layout spec.
func NewGroupSet(spec TrendSpec) *GroupSet {
	return &GroupSet{spec: spec, fp: NewFingerprinter(), groups: map[string]*CauseGroup{}, byClass: ClassCounts{}}
}

// Add puts every record of a route location into its group.
func (s *GroupSet) Add(loc Location, recs []record.Record) {
	for _, r := range recs {
		s.add(loc, r)
	}
}

func (s *GroupSet) add(loc Location, r record.Record) {
	f := s.fp.Of(r.Failure.RouteID, r.Failure)
	g, ok := s.groups[f.ID]
	if !ok {
		g = &CauseGroup{Fingerprint: f.ID, RouteID: loc.Route.ID, RouteName: loc.Route.Name, Step: r.Failure.Step,
			StepLabel: loc.Steps.Of(r.Failure.VersionNo, r.Failure.Branch, r.Failure.Step), ErrorType: r.Failure.ErrorType,
			ErrorClass: r.Failure.ErrorType.Class(), Pattern: f.Pattern, Versions: []int{},
			Trend: Trend{Since: s.spec.Since, BucketSeconds: int64(s.spec.Bucket / time.Second), Counts: make([]int, s.spec.Buckets)}}
		s.groups[f.ID] = g
	}
	g.observe(r, s.spec)
	s.byClass[g.ErrorClass]++
}

// Merge adds the groups of other (another route: fingerprints never collide across routes).
func (s *GroupSet) Merge(other *GroupSet) {
	for id, g := range other.groups {
		s.groups[id] = g
	}
	for c, n := range other.byClass {
		s.byClass[c] += n
	}
}

// Items returns the groups by count desc, then newest first, at most MaxGroups.
func (s *GroupSet) Items() []CauseGroup {
	out := make([]CauseGroup, 0, len(s.groups))
	for _, g := range s.groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Count != out[b].Count {
			return out[a].Count > out[b].Count
		}
		if !out[a].LastSeenAt.Equal(out[b].LastSeenAt) {
			return out[a].LastSeenAt.After(out[b].LastSeenAt)
		}
		return out[a].Fingerprint < out[b].Fingerprint
	})
	return out[:min(len(out), int(MaxGroups))]
}

// ByClass returns the record counts per class.
func (s *GroupSet) ByClass() ClassCounts { return s.byClass }

// GroupPage is the response of the groups endpoints.
type GroupPage struct {
	Items       []CauseGroup   `json:"items"`
	Scanned     int            `json:"scanned"`
	Truncated   bool           `json:"truncated"`
	Approximate bool           `json:"approximate"`
	ByClass     ClassCounts    `json:"byClass"`
	Warnings    []RouteWarning `json:"warnings,omitempty"`
}

// PageOf builds a page from a set and its scan totals.
func (s *GroupSet) PageOf(scanned int, truncated, partial bool) GroupPage {
	return GroupPage{Items: s.Items(), Scanned: scanned, Truncated: truncated, Approximate: truncated || partial, ByClass: s.byClass}
}
