package dlq

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/config"
)

// RouteInfo is what the feature needs to know about a route.
type RouteInfo struct {
	ID              string
	Name            string
	TeamID          string
	ActiveVersionID string
}

// HasActiveVersion reports whether the route is deployed.
func (r RouteInfo) HasActiveVersion() bool { return r.ActiveVersionID != "" }

// Target is one DLQ connection and the DLQ topics on it.
type Target struct {
	ConnectionID string
	Topics       []string
}

// Has reports whether topic belongs to the target.
func (t Target) Has(topic string) bool {
	for _, x := range t.Topics {
		if x == topic {
			return true
		}
	}
	return false
}

// Location is where a route's dead letters live (topics of all versions, grouped by connection).
type Location struct {
	Route    RouteInfo
	Targets  []Target
	Steps    StepLabels
	Formats  SourceFormats
	Delivery config.DeliveryMode
}

// Presenter returns the row presenter of the location (step labels, active delivery mode).
func (l Location) Presenter() Presenter {
	return Presenter{Steps: l.Steps, Delivery: l.Delivery}
}

// TargetOf returns the target holding topic.
func (l Location) TargetOf(topic string) (Target, bool) {
	for _, t := range l.Targets {
		if t.Has(topic) {
			return t, true
		}
	}
	return Target{}, false
}

// VersionConfig is one stored version of a route: id, number and raw config.
type VersionConfig struct {
	ID     string
	No     int
	Config json.RawMessage
}

// RouteSource is the port to the routes store (adapter over internal/routes repositories).
type RouteSource interface {
	Route(ctx context.Context, id string) (RouteInfo, error)
	Routes(ctx context.Context) ([]RouteInfo, error)
	Versions(ctx context.Context, routeID string) ([]VersionConfig, error)
	Version(ctx context.Context, versionID string) (VersionConfig, error)
}

// Locator resolves a route to its DLQ location.
type Locator interface {
	Locate(ctx context.Context, routeID string) (Location, error)
}

// StoreLocator reads DLQ topics and connections over every stored version of a route.
type StoreLocator struct {
	Routes RouteSource
}

// Locate implements Locator; a never-deployed route has no targets.
func (l StoreLocator) Locate(ctx context.Context, routeID string) (Location, error) {
	route, err := l.Routes.Route(ctx, routeID)
	if err != nil {
		return Location{}, err
	}
	versions, err := l.Routes.Versions(ctx, routeID)
	if err != nil {
		return Location{}, err
	}
	return Location{
		Route: route, Targets: TargetSet{}.Of(route, versions), Steps: StepLabelsOf{}.Versions(versions),
		Formats: SourceFormatsOf{}.Versions(versions), Delivery: DeliveryModeOf{}.Active(route, versions),
	}, nil
}

// TargetSet groups the distinct (DLQ connection, DLQ topic) pairs of versions.
type TargetSet struct{}

// Of returns targets sorted by connection with sorted topics; unparsable versions are skipped.
func (TargetSet) Of(route RouteInfo, versions []VersionConfig) []Target {
	byConn := map[string]map[string]bool{}
	for _, v := range versions {
		conn, topic, ok := (DLQAddress{}).Of(route.Name, v.Config)
		if !ok {
			continue
		}
		if byConn[conn] == nil {
			byConn[conn] = map[string]bool{}
		}
		byConn[conn][topic] = true
	}
	out := make([]Target, 0, len(byConn))
	for conn, topics := range byConn {
		t := Target{ConnectionID: conn}
		for topic := range topics {
			t.Topics = append(t.Topics, topic)
		}
		sort.Strings(t.Topics)
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ConnectionID < out[j].ConnectionID })
	return out
}

// DLQAddress reads the DLQ connection and topic of one version config.
type DLQAddress struct{}

// Of returns errorPolicy.dlqConnectionId (default source) and errorPolicy.dlqTopic (default router.dlq.<route>).
func (DLQAddress) Of(routeName string, raw json.RawMessage) (string, string, bool) {
	doc, err := config.Parse(raw)
	if err != nil {
		return "", "", false
	}
	cfg, err := doc.Config()
	if err != nil {
		return "", "", false
	}
	topic := record.TopicNaming{}.Default(routeName)
	if cfg.ErrorPolicy != nil && cfg.ErrorPolicy.DLQTopic != nil && *cfg.ErrorPolicy.DLQTopic != "" {
		topic = *cfg.ErrorPolicy.DLQTopic
	}
	return cfg.DLQConnection(), topic, cfg.DLQConnection() != ""
}
