package overview

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/routes"
)

type fakeLookup struct{ err error }

func (f fakeLookup) Get(_ context.Context, id routes.RouteID) (routes.Route, error) {
	return routes.Route{ID: id}, f.err
}

type downQuerier struct{}

func (downQuerier) Kind() query.BackendKind { return query.BackendMock }
func (downQuerier) Instant(context.Context, query.Expr) (query.Vector, error) {
	return nil, errors.New("connection refused")
}
func (downQuerier) Range(context.Context, query.Expr, query.Window) ([]query.Series, error) {
	return nil, errors.New("connection refused")
}

func newService(q query.Querier, lookup fakeLookup) *RouteOverviewService {
	now := time.Unix(1700000000, 0)
	return &RouteOverviewService{Routes: lookup, Querier: q, Now: func() time.Time { return now }}
}

func TestLoadWithMockBackend(t *testing.T) {
	q := query.NewMockQuerier(func() time.Time { return time.Unix(1700000000, 0) })
	m, err := newService(q, fakeLookup{}).Load(context.Background(), "r1", metrics.Range1h)
	if err != nil {
		t.Fatal(err)
	}
	if m.Range != metrics.Range1h || m.Step != "30s" || len(m.Pipeline) != 1 || m.Pipeline[0].Branch != "main" {
		t.Fatalf("overview %+v", m)
	}
	if len(m.Series.EventsIn) != 121 || len(m.Series.LagByPartition) != query.DefaultMockShape.Partitions || len(m.Series.LatencyP95) == 0 {
		t.Fatalf("series sizes in=%d lag=%d", len(m.Series.EventsIn), len(m.Series.LagByPartition))
	}
	body, _ := json.Marshal(m)
	if !strings.Contains(string(body), `"eventsIn":[[`) {
		t.Fatalf("points must encode as [unixSec,value]: %s", body[:120])
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := newService(downQuerier{}, fakeLookup{}).Load(context.Background(), "r1", metrics.Range15m); !query.IsUnavailable(err) {
		t.Fatalf("want unavailable, got %v", err)
	}
	_, err := newService(downQuerier{}, fakeLookup{err: routes.ErrRouteNotFound}).Load(context.Background(), "r1", metrics.Range15m)
	if !errors.Is(err, routes.ErrRouteNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestPipelineAssembler(t *testing.T) {
	v := func(pairs map[string]float64) query.Vector {
		var out query.Vector
		for b, n := range pairs {
			out = append(out, query.Sample{Labels: query.Labels{"branch": b}, Value: n})
		}
		return out
	}
	single := PipelineAssembler{}.Rows(v(map[string]float64{"main": 100}), v(map[string]float64{"main": 30}),
		v(map[string]float64{"main": 65}), v(map[string]float64{"main": 5}))
	if len(single) != 1 || single[0] != (PipelineRow{Branch: "main", Read: 100, Passed: 70, Sent: 65, DLQ: 5}) {
		t.Fatalf("single %+v", single)
	}
	rows := PipelineAssembler{}.Rows(v(map[string]float64{"main": 100}), v(map[string]float64{"main": 30}),
		v(map[string]float64{"main": 65, "eu": 20}), v(map[string]float64{"main": 5, "eu": 1}))
	if len(rows) != 2 || rows[0] != (PipelineRow{Branch: "eu", Read: 100, Passed: 21, Sent: 20, DLQ: 1}) ||
		rows[1] != (PipelineRow{Branch: "main", Read: 100, Passed: 70, Sent: 65, DLQ: 5}) {
		t.Fatalf("branch rows %+v", rows)
	}
}

func TestPartitionCapSumsTheRest(t *testing.T) {
	at := time.Unix(1700000000, 0)
	var series []query.Series
	for i := 0; i < 5; i++ {
		series = append(series, query.Series{Labels: query.Labels{"partition": strconv.Itoa(i)},
			Points: []query.Point{{At: at, Value: float64(i)}}})
	}
	out := PartitionCap{Max: 2}.Apply(series)
	if len(out) != 3 || out["4"] == nil || out["3"] == nil || out[string(PartitionOther)][0].Value != 3 {
		t.Fatalf("capped %+v", out)
	}
}
