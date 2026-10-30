package retrytopic

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// Producer is the delivery surface tier records are produced through (the runner's delivery strategy).
type Producer interface {
	Produce(ctx context.Context, out delivery.Outgoing, onAck delivery.AckFunc)
}

// TierRetention is how long tier records are kept (same as the DLQ topic).
type TierRetention time.Duration

const DefaultTierRetention TierRetention = TierRetention(7 * 24 * time.Hour)

// Millis returns the retention in milliseconds (topic config).
func (r TierRetention) Millis() int64 { return time.Duration(r).Milliseconds() }

// Failed is the failure being scheduled: its DLQ error type and message.
type Failed struct {
	ErrorType record.ErrorType
	Message   string
}

// Scheduler produces failed records to the tier topic of their next attempt.
type Scheduler struct {
	Planner      Planner
	RouteName    string
	ConnectionID string
	Producer     Producer
	Now          func() time.Time
}

// Schedule writes src as attempt used+1; onAck receives the produce outcome.
func (s Scheduler) Schedule(ctx context.Context, src engine.RawRecord, used int, f Failed, onAck delivery.AckFunc) {
	n, now := used+1, s.Now()
	env := Envelope{Attempt: n, DueAt: s.Planner.DueAt(now, n), ErrorType: f.ErrorType, Message: f.Message, Source: src}
	rec := EnvelopeCodec{}.Encode(env, TierNaming{}.Topic(s.RouteName, n), now)
	s.Producer.Produce(ctx, delivery.Outgoing{Record: rec, ConnectionID: s.ConnectionID, Partitioning: config.PartitioningModeByKey}, onAck)
}

// Topics returns the tier topics of the policy.
func (s Scheduler) Topics() []string {
	return TierNaming{}.Topics(s.RouteName, s.Planner.Policy.MaxAttempts)
}

// TopicSpecs returns the tier topics to create on runner start.
func (s Scheduler) TopicSpecs(partitions int) []kafka.TopicSpec {
	out := []kafka.TopicSpec{}
	for _, t := range s.Topics() {
		out = append(out, kafka.TopicSpec{Name: t, Partitions: int32(max(partitions, 1)), ReplicationFactor: int16(BrokerDefaultReplication),
			RetentionMs: DefaultTierRetention.Millis()})
	}
	return out
}

// ReplicationDefault asks the broker for its default replication factor.
type ReplicationDefault int16

const BrokerDefaultReplication ReplicationDefault = -1
