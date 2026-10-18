package flowmap

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	in "github.com/eventrouter/router/internal/metrics/instruments"
	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/routes"
)

// NodeType is the kind of a flow-map node.
type NodeType string

const (
	NodeConnection NodeType = "connection"
	NodeTopic      NodeType = "topic"
)

// IDPart builds node and edge ids.
type IDPart string

const (
	PrefixConnection IDPart = "conn:"
	PrefixTopic      IDPart = "topic:"
	IDSeparator      IDPart = ":"
)

// RateWindow is the rate window of edge throughput.
type RateWindow time.Duration

const DefaultRateWindow RateWindow = RateWindow(5 * time.Minute)

// Filters are the GET /flow-map query filters (empty = no filter).
type Filters struct {
	Connection string
	Team       string
	Tag        string
}

// Node is a connection or a topic.
type Node struct {
	ID           string   `json:"id"`
	Type         NodeType `json:"type"`
	Label        string   `json:"label"`
	ConnectionID string   `json:"connectionId"`
	EnvTag       string   `json:"envTag,omitempty"`
}

// Edge is one route branch from a source topic to a destination topic.
type Edge struct {
	ID           string        `json:"id"`
	RouteID      string        `json:"routeId"`
	RouteName    string        `json:"routeName"`
	TeamName     string        `json:"teamName"`
	Branch       string        `json:"branch"`
	From         string        `json:"from"`
	To           string        `json:"to"`
	EventsPerSec float64       `json:"eventsPerSec"`
	Status       routes.Status `json:"status"`
}

// FlowMap is the GET /flow-map response.
type FlowMap struct {
	Nodes            []Node `json:"nodes"`
	Edges            []Edge `json:"edges"`
	MetricsAvailable bool   `json:"metricsAvailable"`
}

// FlowMapService assembles the map; a metrics-store outage leaves throughput at 0 (metricsAvailable=false).
type FlowMapService struct {
	Store   Store
	Querier query.Querier
	Log     *slog.Logger
}

// Build returns the filtered map.
func (s *FlowMapService) Build(ctx context.Context, f Filters) (FlowMap, error) {
	rows, err := s.Store.Routes(ctx)
	if err != nil {
		return FlowMap{}, err
	}
	conns, err := s.Store.Connections(ctx)
	if err != nil {
		return FlowMap{}, err
	}
	rates, ok := s.rates(ctx)
	g := &graph{conns: conns, nodes: map[string]Node{}}
	for _, r := range rows {
		if (FilterMatcher{F: f, Conns: conns}).Matches(r) {
			g.addRoute(r, rates[r.ID])
		}
	}
	return FlowMap{Nodes: g.sortedNodes(), Edges: append([]Edge{}, g.edges...), MetricsAvailable: ok}, nil
}

func (s *FlowMapService) rates(ctx context.Context) (map[string]float64, bool) {
	expr := query.ExprCatalog{}.Build(query.QueryEventsInByRoute, query.Params{Rate: time.Duration(DefaultRateWindow)})
	v, err := s.Querier.Instant(ctx, expr)
	if err != nil {
		s.Log.Warn("flow map: throughput unavailable", string(logging.AttrError), err)
		return map[string]float64{}, false
	}
	return v.ByLabel(string(in.LabelRouteID)), true
}

// FilterMatcher applies connection / team / tag filters to one route.
type FilterMatcher struct {
	F     Filters
	Conns map[string]ConnectionRow
}

// Matches reports whether r passes every set filter (team by id or name; tag by route tag or connection env).
func (m FilterMatcher) Matches(r RouteRow) bool {
	ends := RouteEnds{}.Connections(r)
	if m.F.Connection != "" && !slices.Contains(ends, m.F.Connection) {
		return false
	}
	if m.F.Team != "" && m.F.Team != r.TeamID && !strings.EqualFold(m.F.Team, r.TeamName) {
		return false
	}
	if m.F.Tag == "" || slices.Contains(r.Tags, m.F.Tag) {
		return true
	}
	for _, id := range ends {
		if m.Conns[id].EnvTag == m.F.Tag {
			return true
		}
	}
	return false
}

// RouteEnds reads source and destination connections of a route config.
type RouteEnds struct{}

// Connections returns the source connection followed by every branch destination connection.
func (RouteEnds) Connections(r RouteRow) []string {
	out := []string{r.Config.Source.ConnectionID}
	for _, b := range r.Config.Branches {
		out = append(out, b.Destination.ConnectionID)
	}
	return out
}

// graph accumulates nodes and edges.
type graph struct {
	conns map[string]ConnectionRow
	nodes map[string]Node
	edges []Edge
}

func (g *graph) addRoute(r RouteRow, rate float64) {
	src := r.Config.Source
	for _, b := range r.Config.Branches {
		to := g.topic(b.Destination.ConnectionID, b.Destination.Topic)
		for _, t := range src.Topics {
			from := g.topic(src.ConnectionID, t)
			g.edges = append(g.edges, Edge{ID: r.ID + string(IDSeparator) + b.ID + string(IDSeparator) + t, RouteID: r.ID,
				RouteName: r.Name, TeamName: r.TeamName, Branch: b.ID, From: from, To: to, EventsPerSec: rate, Status: r.Status})
		}
	}
}

// topic registers the topic node and its connection node, returning the topic node id.
func (g *graph) topic(connID, topic string) string {
	c := g.conns[connID]
	label := c.Name
	if label == "" {
		label = connID
	}
	cid := string(PrefixConnection) + connID
	g.nodes[cid] = Node{ID: cid, Type: NodeConnection, Label: label, ConnectionID: connID, EnvTag: c.EnvTag}
	tid := string(PrefixTopic) + connID + string(IDSeparator) + topic
	g.nodes[tid] = Node{ID: tid, Type: NodeTopic, Label: topic, ConnectionID: connID, EnvTag: c.EnvTag}
	return tid
}

func (g *graph) sortedNodes() []Node {
	out := make([]Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		out = append(out, n)
	}
	slices.SortFunc(out, func(a, b Node) int { return strings.Compare(a.ID, b.ID) })
	return out
}
