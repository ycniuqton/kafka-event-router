package routelist

import (
	"context"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/routes"
)

func TestSummariesFromMockBackend(t *testing.T) {
	now := func() time.Time { return time.Unix(1700000000, 0) }
	r := &PromRouteMetricsReader{Querier: query.NewMockQuerier(now), Now: now}
	got, err := r.Summaries(context.Background(), []routes.RouteID{"a", "b"})
	if err != nil || len(got) != 2 {
		t.Fatalf("summaries %+v %v", got, err)
	}
	s := got["a"]
	if s.EventsPerSec <= 0 || len(s.Sparkline) != 12 || s.MatchPct < 0 || s.MatchPct > 100 {
		t.Fatalf("summary %+v", s)
	}
	if r.Kind() != ReaderKind {
		t.Fatal("kind")
	}
	empty, _ := r.Summaries(context.Background(), nil)
	if len(empty) != 0 {
		t.Fatal("empty ids must not query")
	}
}

func TestMatchRate(t *testing.T) {
	cases := []struct{ consumed, filtered, want float64 }{{0, 0, 100}, {200, 50, 75}, {3, 1, 66.7}, {10, 12, 0}}
	for _, c := range cases {
		if got := (MatchRate{}).Of(c.consumed, c.filtered); got != c.want {
			t.Errorf("MatchRate(%v,%v) = %v, want %v", c.consumed, c.filtered, got, c.want)
		}
	}
}
