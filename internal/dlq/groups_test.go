package dlq

import (
	"fmt"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
)

func groupRecord(i int, et record.ErrorType, msg string, version int) record.Record {
	r := failure(i, et)
	r.Ref = Ref{Topic: testDLQTopic, Offset: int64(i)}
	r.Failure.Message, r.Failure.VersionNo = msg, version
	return r
}

func TestTrendSpec(t *testing.T) {
	now := baseTime.Add(24 * time.Hour)
	spec := TrendSpecOf{}.For(Query{}, now)
	if spec.Buckets != 24 || spec.Bucket != time.Hour || !spec.Since.Equal(baseTime) {
		t.Fatalf("default spec %+v", spec)
	}
	if i, ok := spec.Index(baseTime.Add(90 * time.Minute)); !ok || i != 1 {
		t.Fatalf("index %d %v", i, ok)
	}
	if _, ok := spec.Index(baseTime.Add(-time.Minute)); ok {
		t.Fatal("before window counted")
	}
	if _, ok := spec.Index(now); ok {
		t.Fatal("window end counted")
	}
	q := Query{Since: baseTime, Until: baseTime.Add(48 * time.Minute)}
	if s := (TrendSpecOf{}).For(q, now); s.Bucket != 2*time.Minute {
		t.Fatalf("query spec %+v", s)
	}
}

func TestGroupSetAggregatesCauses(t *testing.T) {
	loc := Location{Route: RouteInfo{ID: "r1", Name: "orders"}}
	recs := []record.Record{}
	for i := 0; i < 5; i++ {
		recs = append(recs, groupRecord(i, record.ErrorStep, fmt.Sprintf("customer ord_%d has no items", i*17), 1+i%2))
	}
	recs = append(recs, groupRecord(10, record.ErrorInternal, "panic: nil map", 2), groupRecord(11, record.ErrorTimeout, "step timed out after 500 ms", 2))
	set := NewGroupSet(TrendSpecOf{}.For(Query{}, baseTime.Add(12*time.Hour)))
	set.Add(loc, recs)
	items := set.Items()
	if len(items) != 3 {
		t.Fatalf("groups %d", len(items))
	}
	top := items[0]
	if top.Count != 5 || top.Pattern != "customer <id> has no items" || top.ErrorClass != record.ClassData || top.RouteName != "orders" {
		t.Fatalf("top %+v", top)
	}
	if len(top.Versions) != 2 || top.Versions[0] != 1 || !top.FirstSeenAt.Equal(baseTime) || !top.LastSeenAt.Equal(baseTime.Add(4*time.Minute)) {
		t.Fatalf("seen/versions %+v", top)
	}
	if top.Sample.ID != (RefCodec{}).Encode(recs[4].Ref) || top.Trend.Counts[12] != 5 || top.Trend.BucketSeconds != 3600 {
		t.Fatalf("sample/trend %+v", top)
	}
	by := set.ByClass()
	if by[record.ClassData] != 5 || by[record.ClassRouter] != 1 || by[record.ClassTransient] != 1 {
		t.Fatalf("by class %+v", by)
	}
}

func TestGroupSetOrderingAndMerge(t *testing.T) {
	spec := TrendSpecOf{}.For(Query{}, baseTime)
	a, b := NewGroupSet(spec), NewGroupSet(spec)
	a.Add(Location{Route: RouteInfo{ID: "r1"}}, []record.Record{groupRecord(1, record.ErrorStep, "x", 1)})
	other := groupRecord(2, record.ErrorStep, "y", 1)
	other.Failure.RouteID = "r2"
	b.Add(Location{Route: RouteInfo{ID: "r2"}}, []record.Record{other, other})
	a.Merge(b)
	items := a.Items()
	if len(items) != 2 || items[0].RouteID != "r2" || items[0].Count != 2 || a.ByClass()[record.ClassData] != 3 {
		t.Fatalf("merged %+v", items)
	}
	page := a.PageOf(3, false, true)
	if !page.Approximate || page.Scanned != 3 {
		t.Fatalf("page %+v", page)
	}
}

func TestQueryMatchesClassAndFingerprint(t *testing.T) {
	r := groupRecord(1, record.ErrorSandboxCrash, "sandbox crashed in worker 3", 1)
	fp := NewFingerprinter().Of(r.Failure.RouteID, r.Failure).ID
	cases := []struct {
		q    Query
		want bool
	}{
		{Query{Classes: []record.ErrorClass{record.ClassRouter}}, true},
		{Query{Classes: []record.ErrorClass{record.ClassData, record.ClassTransient}}, false},
		{Query{Fingerprint: fp}, true},
		{Query{Fingerprint: "0000000000000000"}, false},
	}
	for i, c := range cases {
		if c.q.Matches(r) != c.want {
			t.Errorf("case %d", i)
		}
	}
	if (Presenter{}).Item(RouteInfo{ID: "r1"}, r, nil).Cause != fp {
		t.Fatal("row fingerprint differs from the filter fingerprint")
	}
}

func TestMemoryGroupCacheTTL(t *testing.T) {
	now := baseTime
	c := NewMemoryGroupCache(func() time.Time { return now })
	c.Set("k", GroupPage{Scanned: 7})
	if p, ok := c.Get("k"); !ok || p.Scanned != 7 {
		t.Fatal("miss")
	}
	now = now.Add(GroupCacheTTL.Duration() + time.Second)
	if _, ok := c.Get("k"); ok {
		t.Fatal("stale hit")
	}
}
