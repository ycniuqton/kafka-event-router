package reconcile

import (
	"encoding/json"
	"slices"

	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/pkg/config"
)

// ReloadClassifier decides how a runner moves from prev to next.
type ReloadClassifier interface {
	Classify(prev, next *config.RouteConfig) domain.ReloadKind
}

// restartIdentity is everything that cannot change without leaving the consumer group or reopening clients.
type restartIdentity struct {
	SourceConnection string
	Topics           []string
	Pattern          string
	Group            string
	Delivery         config.DeliveryMode
	Destinations     []string
	DLQ              failures.DLQTarget
}

// ConfigReloadClassifier compares restart identities, then whole documents.
type ConfigReloadClassifier struct {
	DLQ failures.DLQTargetResolver
}

// Classify implements ReloadClassifier: source / destination connection / delivery / DLQ change → full
// restart; any other difference (filter, transform, limits, formats) → pipeline swap; identical → none.
func (c ConfigReloadClassifier) Classify(prev, next *config.RouteConfig) domain.ReloadKind {
	if !(JSONEquality{}).Equal(c.identity(prev), c.identity(next)) {
		return domain.ReloadFullRestart
	}
	if (JSONEquality{}).Equal(prev, next) {
		return domain.ReloadNone
	}
	return domain.ReloadPipelineSwap
}

func (c ConfigReloadClassifier) identity(cfg *config.RouteConfig) restartIdentity {
	id := restartIdentity{SourceConnection: cfg.Source.ConnectionID, Topics: slices.Sorted(slices.Values(cfg.Source.Topics)),
		Delivery: delivery.DeliveryModes{}.Of(cfg), DLQ: c.DLQ.Resolve(cfg)}
	if cfg.Source.TopicPattern != nil {
		id.Pattern = *cfg.Source.TopicPattern
	}
	if cfg.Source.ConsumerGroup != nil {
		id.Group = *cfg.Source.ConsumerGroup
	}
	for _, b := range cfg.Branches {
		id.Destinations = append(id.Destinations, b.Destination.ConnectionID)
	}
	slices.Sort(id.Destinations)
	id.Destinations = slices.Compact(id.Destinations)
	return id
}

// JSONEquality compares values by their JSON encoding.
type JSONEquality struct{}

// Equal reports whether a and b encode identically.
func (JSONEquality) Equal(a, b any) bool {
	ra, errA := json.Marshal(a)
	rb, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(ra) == string(rb)
}
