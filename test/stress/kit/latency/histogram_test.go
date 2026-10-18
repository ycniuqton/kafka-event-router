package latency

import (
	"math"
	"testing"
	"time"
)

func TestQuantilesOfUniformMillis(t *testing.T) {
	h := New()
	for i := 1; i <= 1000; i++ {
		h.Record(time.Duration(i) * time.Millisecond)
	}
	s := h.Summary()
	cases := map[string][2]float64{"p50": {s.P50Ms, 500}, "p95": {s.P95Ms, 950}, "p99": {s.P99Ms, 990}, "max": {s.MaxMs, 1000}}
	for name, c := range cases {
		if rel := math.Abs(c[0]-c[1]) / c[1]; rel > 0.016 {
			t.Errorf("%s = %.2f ms, want %.0f ±1.6%%", name, c[0], c[1])
		}
	}
	if s.Count != 1000 || s.MinMs != 1 {
		t.Errorf("count/min = %d/%.3f", s.Count, s.MinMs)
	}
}

func TestSmallValuesAreExact(t *testing.T) {
	h := New()
	for _, us := range []int64{0, 1, 5, 127} {
		h.Record(time.Duration(us) * time.Microsecond)
	}
	if got := h.Quantile(1); got != 127*time.Microsecond {
		t.Errorf("max quantile = %v", got)
	}
	if got := h.Quantile(0.25); got != 0 {
		t.Errorf("p25 = %v", got)
	}
}

func TestBucketBoundsAreMonotonicAndContainValue(t *testing.T) {
	prev := int64(-1)
	for i := 0; i < Geometry.Buckets-64; i++ {
		u := Geometry.upper(i)
		if u <= prev {
			t.Fatalf("bucket %d upper %d not above %d", i, u, prev)
		}
		prev = u
	}
	for _, us := range []int64{128, 129, 255, 256, 1_000_000, 3_600_000_000} {
		if up := Geometry.upper(Geometry.index(us)); up < us || float64(up-us)/float64(us) > 0.016 {
			t.Errorf("value %d -> upper %d", us, up)
		}
	}
}

func TestNegativeLatencyCountsAsSkew(t *testing.T) {
	h := New()
	h.Record(-time.Millisecond)
	if s := h.Summary(); s.Skewed != 1 || s.MaxMs != 0 {
		t.Errorf("summary = %+v", s)
	}
}

func TestSparseRoundTripMerges(t *testing.T) {
	a, b := New(), New()
	for i := 0; i < 100; i++ {
		a.Record(time.Duration(i) * time.Millisecond)
		b.Record(time.Duration(i+100) * time.Millisecond)
	}
	merged := New()
	merged.MergeSparse(a.Sparse())
	merged.MergeSparse(b.Sparse())
	direct := New()
	direct.Merge(a)
	direct.Merge(b)
	if merged.Summary() != direct.Summary() {
		t.Errorf("sparse %+v != direct %+v", merged.Summary(), direct.Summary())
	}
	if merged.Count() != 200 || merged.Summary().MaxMs != 199 {
		t.Errorf("merged = %+v", merged.Summary())
	}
}

func TestEmptySummaryIsZero(t *testing.T) {
	if s := New().Summary(); s != (Summary{}) {
		t.Errorf("empty = %+v", s)
	}
}
