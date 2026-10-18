package runner

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/eventrouter/router/test/stress/kit/kafkaops"
	"github.com/eventrouter/router/test/stress/kit/platform"
	"github.com/eventrouter/router/test/stress/kit/scenario"
)

// Naming builds the stress- names of one run: stress-<scenario short>-<stamp>-<case>-r<i>-in|out|dlq|g.
type Naming struct {
	Base string
}

// Suffixes of per-route resources.
var Suffixes = struct{ In, Out, DLQ, Group, Conn string }{In: "in", Out: "out", DLQ: "dlq", Group: "g", Conn: "conn"}

// NewNaming derives a unique base from the scenario name (its first token, e.g. "s2").
func NewNaming(scenarioName string) Naming {
	short, _, _ := strings.Cut(scenarioName, "-")
	stamp := time.Now().UTC().Format("0102150405")
	return Naming{Base: fmt.Sprintf("%s%s-%s%02d", kafkaops.StressGuard.Prefix, short, stamp, rand.IntN(100))}
}

// Connection is the run's connection name.
func (n Naming) Connection() string {
	return n.Base + "-" + Suffixes.Conn
}

// Routes returns the topics of count routes of a case starting at index from, per topology.
func (n Naming) Routes(caseName string, topology scenario.Topology, from, count int) []platform.RouteTopics {
	shared := fmt.Sprintf("%s-%s-%s", n.Base, caseName, Suffixes.In)
	out := make([]platform.RouteTopics, 0, count)
	for i := from; i < from+count; i++ {
		route := fmt.Sprintf("%s-%s-r%d", n.Base, caseName, i)
		input := route + "-" + Suffixes.In
		if topology == scenario.TopologySharedInput {
			input = shared
		}
		out = append(out, platform.RouteTopics{Name: route, Input: input, Output: route + "-" + Suffixes.Out, DLQ: route + "-" + Suffixes.DLQ, Group: route + "-" + Suffixes.Group})
	}
	return out
}

// Registry records every resource a run creates, for cleanup in reverse dependency order.
type Registry struct {
	mu          sync.Mutex
	routes      []string
	topics      []string
	groups      []string
	connections []string
	seenTopics  map[string]bool
}

// Route records a route id.
func (r *Registry) Route(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = append(r.routes, id)
}

// Topics records topic names.
func (r *Registry) Topics(names ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seenTopics == nil {
		r.seenTopics = map[string]bool{}
	}
	for _, n := range names {
		if !r.seenTopics[n] {
			r.seenTopics[n] = true
			r.topics = append(r.topics, n)
		}
	}
}

// Group records consumer groups.
func (r *Registry) Group(names ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups = append(r.groups, names...)
}

// Connection records a connection id.
func (r *Registry) Connection(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connections = append(r.connections, id)
}

// CleanupReport lists what could not be removed.
type CleanupReport struct {
	ArchivedRoutes int      `json:"archivedRoutes"`
	DeletedTopics  int      `json:"deletedTopics"`
	Errors         []string `json:"errors,omitempty"`
}

// Cleanup archives routes, then deletes groups (retried while workers still hold them), topics and connections;
// only stress- names ever reach Kafka deletes.
func (r *Registry) Cleanup(ctx context.Context, p *platform.Platform, adm KafkaAdmin) CleanupReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	var rep CleanupReport
	for _, id := range r.routes {
		if err := p.Archive(ctx, id); err != nil {
			rep.Errors = append(rep.Errors, err.Error())
			continue
		}
		rep.ArchivedRoutes++
	}
	if err := r.deleteGroups(ctx, adm); err != nil {
		rep.Errors = append(rep.Errors, err.Error())
	}
	if err := adm.DeleteTopics(ctx, r.topics...); err != nil {
		rep.Errors = append(rep.Errors, err.Error())
	} else {
		rep.DeletedTopics = len(r.topics)
	}
	for _, id := range r.connections {
		if err := p.DeleteConnection(ctx, id); err != nil {
			rep.Errors = append(rep.Errors, err.Error())
		}
	}
	r.routes, r.topics, r.groups, r.connections, r.seenTopics = nil, nil, nil, nil, nil
	return rep
}

func (r *Registry) deleteGroups(ctx context.Context, adm KafkaAdmin) error {
	var err error
	for i := 0; i < CleanupRetry.Attempts; i++ {
		if err = adm.DeleteGroups(ctx, r.groups...); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(CleanupRetry.Pause):
		}
	}
	return err
}
