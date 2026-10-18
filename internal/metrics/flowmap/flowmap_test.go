package flowmap

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/pkg/config"
)

type memStore struct{}

func route(id, team string, tags []string, src, dst string, status routes.Status) RouteRow {
	return RouteRow{ID: id, Name: "route-" + id, TeamID: team, TeamName: "team-" + team, Tags: tags, Status: status,
		Config: config.RouteConfig{Source: config.Source{ConnectionID: src, Topics: []string{"in-a", "in-b"}},
			Branches: []config.Branch{{ID: "main", Destination: config.Destination{ConnectionID: dst, Topic: "out"}},
				{ID: "audit", Destination: config.Destination{ConnectionID: dst, Topic: "audit"}}}}}
}

func (memStore) Routes(context.Context) ([]RouteRow, error) {
	return []RouteRow{
		route("r1", "t1", []string{"orders"}, "c-prod", "c-prod", routes.StatusRunning),
		route("r2", "t2", nil, "c-dev", "c-dev", routes.StatusError),
	}, nil
}

func (memStore) Connections(context.Context) (map[string]ConnectionRow, error) {
	return map[string]ConnectionRow{"c-prod": {ID: "c-prod", Name: "prod", EnvTag: "prod"}, "c-dev": {ID: "c-dev", Name: "dev", EnvTag: "dev"}}, nil
}

type rates struct{ err error }

func (rates) Kind() query.BackendKind { return query.BackendMock }
func (r rates) Instant(context.Context, query.Expr) (query.Vector, error) {
	return query.Vector{{Labels: query.Labels{"route_id": "r1"}, Value: 142}}, r.err
}
func (rates) Range(context.Context, query.Expr, query.Window) ([]query.Series, error) {
	return nil, nil
}

func service(err error) *FlowMapService {
	return &FlowMapService{Store: memStore{}, Querier: rates{err: err}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestFlowMapEdgesPerBranchAndTopic(t *testing.T) {
	m, err := service(nil).Build(context.Background(), Filters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Edges) != 8 || !m.MetricsAvailable {
		t.Fatalf("2 routes × 2 source topics × 2 branches: %d", len(m.Edges))
	}
	e := m.Edges[0]
	if e.RouteID != "r1" || e.EventsPerSec != 142 || e.From != "topic:c-prod:in-a" || e.To != "topic:c-prod:out" || e.Branch != "main" {
		t.Fatalf("edge: %+v", e)
	}
	if m.Edges[7].Status != routes.StatusError || m.Edges[7].EventsPerSec != 0 {
		t.Fatalf("error route: %+v", m.Edges[7])
	}
	if len(m.Nodes) != 10 || m.Nodes[0].Type != NodeConnection {
		t.Fatalf("nodes: %d %+v", len(m.Nodes), m.Nodes[0])
	}
}

func TestFlowMapFilters(t *testing.T) {
	cases := []struct {
		f    Filters
		want int
	}{
		{Filters{Connection: "c-dev"}, 4},
		{Filters{Team: "t1"}, 4},
		{Filters{Team: "TEAM-T2"}, 4},
		{Filters{Tag: "orders"}, 4},
		{Filters{Tag: "prod"}, 4},
		{Filters{Tag: "dev", Team: "t1"}, 0},
	}
	for _, c := range cases {
		m, _ := service(nil).Build(context.Background(), c.f)
		if len(m.Edges) != c.want {
			t.Errorf("%+v: %d edges", c.f, len(m.Edges))
		}
	}
}

func TestFlowMapMetricsDown(t *testing.T) {
	m, err := service(errors.New("down")).Build(context.Background(), Filters{})
	if err != nil || m.MetricsAvailable || m.Edges[0].EventsPerSec != 0 {
		t.Fatalf("degraded: %v %+v", err, m)
	}
}
