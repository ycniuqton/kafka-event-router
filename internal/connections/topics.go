package connections

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/eventrouter/router/pkg/kafka"
)

// TopicView is one row of the topic browser; RatePerSec is nil until measurable.
type TopicView struct {
	Name              string   `json:"name"`
	Partitions        int      `json:"partitions"`
	ReplicationFactor int      `json:"replicationFactor"`
	Internal          bool     `json:"internal"`
	RatePerSec        *float64 `json:"ratePerSec"`
}

// TopicQuery filters the topic list.
type TopicQuery struct {
	Text            string
	IncludeInternal bool
}

// RateEstimator turns end-offset snapshots into per-topic message rates; failures only hide rates.
type RateEstimator interface {
	Rates(ctx context.Context, connID uuid.UUID, ends map[string]int64) (map[string]*float64, error)
}

// ClusterFailures converts cluster errors into 502 problems with plain-language hints.
type ClusterFailures struct {
	Classifier *kafka.FailureClassifier
	Hints      *HintCatalog
}

// Wrap classifies err for a step context (e.g. listing topics).
func (f ClusterFailures) Wrap(step TestStep, err error) error {
	reason := f.Classifier.Classify(err)
	h := f.Hints.Lookup(step, reason)
	return &ClusterUnavailableError{Reason: reason, Message: h.Message, Hint: h.Hint, Cause: err}
}

// InternalTopicRule decides which topics are hidden by default (router DLQ topics stay visible).
type InternalTopicRule struct{}

// Hidden reports whether a topic is internal.
func (InternalTopicRule) Hidden(t kafka.TopicInfo) bool {
	if strings.HasPrefix(t.Name, string(RouterDLQPrefix)) {
		return false
	}
	return t.Internal || strings.HasPrefix(t.Name, string(InternalPrefix)) || t.Name == string(SchemasTopic)
}

// TopicBrowser lists a cluster's topics with partitions and rate.
type TopicBrowser struct {
	resolver *SpecResolver
	clients  ClientOpener
	rates    RateEstimator
	failures ClusterFailures
	internal InternalTopicRule
}

// NewTopicBrowser builds the browser.
func NewTopicBrowser(resolver *SpecResolver, clients ClientOpener, rates RateEstimator, failures ClusterFailures) *TopicBrowser {
	return &TopicBrowser{resolver: resolver, clients: clients, rates: rates, failures: failures}
}

// List returns the filtered topics of a connection; cluster errors become ClusterUnavailableError.
func (b *TopicBrowser) List(ctx context.Context, id uuid.UUID, q TopicQuery) ([]TopicView, error) {
	_, spec, err := b.resolver.Resolve(ctx, id)
	if err != nil {
		return nil, err
	}
	client, err := b.clients.NewClient(ctx, spec, kafka.PurposeAdmin)
	if err != nil {
		return nil, b.failures.Wrap(StepAuthenticated, err)
	}
	defer client.Close()
	topics, err := client.Admin().ListTopics(ctx)
	if err != nil {
		return nil, b.failures.Wrap(StepListTopics, err)
	}
	visible := b.filter(topics, q)
	rates, err := b.ratesOf(ctx, id, client.Admin(), visible)
	if err != nil {
		return nil, err
	}
	return b.views(visible, rates), nil
}

// TopicExists reports whether topic exists on the connection (routes' destination-topic deploy check).
func (b *TopicBrowser) TopicExists(ctx context.Context, connectionID, topic string) (bool, error) {
	id, err := uuid.Parse(connectionID)
	if err != nil {
		return false, err
	}
	topics, err := b.List(ctx, id, TopicQuery{Text: topic, IncludeInternal: true})
	if err != nil {
		return false, err
	}
	for _, t := range topics {
		if t.Name == topic {
			return true, nil
		}
	}
	return false, nil
}

func (b *TopicBrowser) filter(topics []kafka.TopicInfo, q TopicQuery) []kafka.TopicInfo {
	text := strings.ToLower(strings.TrimSpace(q.Text))
	out := make([]kafka.TopicInfo, 0, len(topics))
	for _, t := range topics {
		if !q.IncludeInternal && b.internal.Hidden(t) {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(t.Name), text) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (b *TopicBrowser) ratesOf(ctx context.Context, id uuid.UUID, admin kafka.Admin, topics []kafka.TopicInfo) (map[string]*float64, error) {
	if len(topics) == 0 {
		return map[string]*float64{}, nil
	}
	names := make([]string, 0, len(topics))
	for _, t := range topics {
		names = append(names, t.Name)
	}
	ends, err := admin.EndOffsets(ctx, names...)
	if err != nil {
		return nil, b.failures.Wrap(StepListTopics, err)
	}
	rates, err := b.rates.Rates(ctx, id, ends)
	if err != nil {
		return map[string]*float64{}, nil
	}
	return rates, nil
}

func (b *TopicBrowser) views(topics []kafka.TopicInfo, rates map[string]*float64) []TopicView {
	out := make([]TopicView, 0, len(topics))
	for _, t := range topics {
		out = append(out, TopicView{Name: t.Name, Partitions: t.Partitions, ReplicationFactor: t.ReplicationFactor, Internal: b.internal.Hidden(t), RatePerSec: rates[t.Name]})
	}
	return out
}
