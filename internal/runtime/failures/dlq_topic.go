package failures

import (
	"context"
	"errors"
	"fmt"

	"github.com/twmb/franz-go/pkg/kerr"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/kafka"
)

// ReplicationDefault asks the broker for its default replication factor.
type ReplicationDefault int16

const BrokerDefaultReplication ReplicationDefault = -1

// TopicPurpose is why a runner creates a topic: the label of its messages and the halt reason when it cannot.
type TopicPurpose struct {
	Label  string
	Reason domain.HaltReason
}

// TopicPurposes are the topics a runner creates on start.
var TopicPurposes = struct{ DLQ, Destination TopicPurpose }{
	DLQ:         TopicPurpose{Label: "DLQ topic", Reason: domain.HaltDLQTopicUnavailable},
	Destination: TopicPurpose{Label: "Destination topic", Reason: domain.HaltDestinationTopicUnavailable},
}

// TopicEnsurer creates missing topics on runner start: the DLQ topic (cleanup delete, 7 days) and destination
// topics of branches with destination.createTopic.
type TopicEnsurer struct{}

// Ensure makes sure the DLQ topic exists on admin (see EnsureSpec).
func (e TopicEnsurer) Ensure(ctx context.Context, admin kafka.Admin, topic string, partitions int) error {
	spec := kafka.TopicSpec{Name: topic, Partitions: int32(max(partitions, 1)), ReplicationFactor: int16(BrokerDefaultReplication),
		RetentionMs: domain.DLQRetentionDays.Millis()}
	return e.EnsureSpec(ctx, admin, spec, TopicPurposes.DLQ)
}

// EnsureSpec creates spec when the topic is missing; a permanent failure (ACL, invalid config) is a
// destination_permanent halt naming the topic, a transient one is retried with the next reconcile.
func (TopicEnsurer) EnsureSpec(ctx context.Context, admin kafka.Admin, spec kafka.TopicSpec, purpose TopicPurpose) error {
	infos, err := admin.DescribeTopics(ctx, spec.Name)
	if err == nil && len(infos) > 0 && infos[0].Partitions > 0 {
		return nil
	}
	_, err = admin.CreateTopic(ctx, spec)
	if err == nil || errors.Is(err, kerr.TopicAlreadyExists) {
		return nil
	}
	if (KafkaErrors{}).Destination(err) == domain.CategoryDestinationTransient {
		return fmt.Errorf("%s %q not ready: %w", purpose.Label, spec.Name, err)
	}
	return &HaltError{Reason: purpose.Reason, Category: domain.CategoryDestinationPermanent,
		Message: fmt.Sprintf("%s %q cannot be created: %v", purpose.Label, spec.Name, err)}
}

// DestinationTopicSpec is the topic a branch destination asks to create (destination.createTopic).
type DestinationTopicSpec struct{}

// Of returns the spec of dst (broker default replication, retention when set).
func (DestinationTopicSpec) Of(dst config.Destination) kafka.TopicSpec {
	spec := kafka.TopicSpec{Name: dst.Topic, Partitions: int32(max(dst.CreateTopic.Partitions, 1)), ReplicationFactor: int16(BrokerDefaultReplication)}
	if dst.CreateTopic.RetentionMs != nil {
		spec.RetentionMs = int64(*dst.CreateTopic.RetentionMs)
	}
	return spec
}
