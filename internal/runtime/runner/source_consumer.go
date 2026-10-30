package runner

import (
	"context"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/kafka"
)

// ClientOpener opens franz-go clients for connection specs.
type ClientOpener interface {
	Open(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.Client, error)
	Session(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose, extra ...kgo.Opt) (*kgo.GroupTransactSession, error)
	Admin(ctx context.Context, spec kafka.ConnectionSpec) (kafka.Client, error)
}

// RebalanceListener receives partition revocations and losses of the source consumer.
type RebalanceListener interface {
	Revoked(ctx context.Context, tps []TopicPartition)
	Lost(tps []TopicPartition)
}

// StartStrategy is the base of every source.startFrom mode (applied only without a committed offset).
type StartStrategy interface {
	Mode() config.StartMode
	Offset(sf *config.StartFrom) kgo.Offset
}

// EarliestStart reads from the beginning.
type EarliestStart struct{}

func (EarliestStart) Mode() config.StartMode              { return config.StartModeEarliest }
func (EarliestStart) Offset(*config.StartFrom) kgo.Offset { return kgo.NewOffset().AtStart() }

// LatestStart reads only new records.
type LatestStart struct{}

func (LatestStart) Mode() config.StartMode              { return config.StartModeLatest }
func (LatestStart) Offset(*config.StartFrom) kgo.Offset { return kgo.NewOffset().AtEnd() }

// TimestampStart reads from the first record at or after startFrom.at.
type TimestampStart struct{}

func (TimestampStart) Mode() config.StartMode { return config.StartModeTimestamp }
func (TimestampStart) Offset(sf *config.StartFrom) kgo.Offset {
	if sf == nil || sf.At == nil {
		return kgo.NewOffset().AtEnd()
	}
	return kgo.NewOffset().AfterMilli(sf.At.UnixMilli())
}

// StartPositions selects the start strategy by mode; explicit offsets are not group-compatible and read from the start.
type StartPositions struct {
	byMode map[config.StartMode]StartStrategy
}

// NewStartPositions registers the strategies; offsets mode maps to earliest.
func NewStartPositions() StartPositions {
	m := map[config.StartMode]StartStrategy{}
	for _, s := range []StartStrategy{EarliestStart{}, LatestStart{}, TimestampStart{}} {
		m[s.Mode()] = s
	}
	m[config.StartModeOffsets] = EarliestStart{}
	return StartPositions{byMode: m}
}

// Of returns the reset offset of sf (absent = latest).
func (p StartPositions) Of(sf *config.StartFrom) kgo.Offset {
	mode := config.StartModeLatest
	if sf != nil && sf.Mode != "" {
		mode = sf.Mode
	}
	s, ok := p.byMode[mode]
	if !ok {
		s = LatestStart{}
	}
	return s.Offset(sf)
}

// SourceConsumer is the group consumer of one runner: rebalance blocked while records are handed to lanes.
// For exactly_once it is a transact session that also produces the route's outputs.
type SourceConsumer struct {
	client  *kgo.Client
	session *kgo.GroupTransactSession
}

// OpenSourceConsumer opens the consumer of v's source with manual commits and rebalance callbacks,
// through the source mode of the route's delivery mode, with fetch buffering sized by fetch.
func OpenSourceConsumer(ctx context.Context, opener ClientOpener, v CompiledVersion, l RebalanceListener, mode SourceMode, fetch SourceFetch) (*SourceConsumer, error) {
	src := v.Config.Source
	opts := []kgo.Opt{
		kgo.ConsumerGroup(ConsumerGroups{}.Of(v)),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxWait(time.Duration(SourceFetchMaxWait)),
		kgo.BlockRebalanceOnPoll(),
		kgo.ConsumeResetOffset(NewStartPositions().Of(src.StartFrom)),
		kgo.OnPartitionsRevoked(func(ctx context.Context, _ *kgo.Client, m map[string][]int32) {
			l.Revoked(ctx, TopicPartitions{}.Of(m))
		}),
		kgo.OnPartitionsLost(func(_ context.Context, _ *kgo.Client, m map[string][]int32) { l.Lost(TopicPartitions{}.Of(m)) }),
	}
	opts = append(opts, TopicSelection{}.Options(src)...)
	opts = append(opts, fetch.Options(v)...)
	return mode.Open(ctx, opener, v, opts)
}

// Client exposes the franz-go client (commits, pause/resume).
func (c *SourceConsumer) Client() *kgo.Client { return c.client }

// Session exposes the transact session (nil unless exactly_once).
func (c *SourceConsumer) Session() *kgo.GroupTransactSession { return c.session }

// Poll returns the next batch of fetches.
func (c *SourceConsumer) Poll(ctx context.Context) kgo.Fetches {
	return c.client.PollRecords(ctx, int(MaxPollRecords))
}

// AllowRebalance lets a pending rebalance proceed once polled records are handed off.
func (c *SourceConsumer) AllowRebalance() { c.client.AllowRebalance() }

// Close leaves the group and closes the client.
func (c *SourceConsumer) Close() { c.client.CloseAllowingRebalance() }

// ConsumerGroups names route consumer groups.
type ConsumerGroups struct{}

// Of returns source.consumerGroup or the default group of the route.
func (ConsumerGroups) Of(v CompiledVersion) string {
	return domain.GroupNaming{}.Of(v.Ref.RouteID, v.Config.Source.ConsumerGroup)
}

// TopicSelection maps source.topics / source.topicPattern to consume options.
type TopicSelection struct{}

// Options returns ConsumeTopics, plus ConsumeRegex for a pattern.
func (TopicSelection) Options(src config.Source) []kgo.Opt {
	if src.TopicPattern != nil && *src.TopicPattern != "" {
		return []kgo.Opt{kgo.ConsumeTopics(*src.TopicPattern), kgo.ConsumeRegex()}
	}
	return []kgo.Opt{kgo.ConsumeTopics(src.Topics...)}
}

// TopicPartitions converts franz-go partition maps.
type TopicPartitions struct{}

// Of flattens a topic → partitions map.
func (TopicPartitions) Of(m map[string][]int32) []TopicPartition {
	out := []TopicPartition{}
	for t, ps := range m {
		for _, p := range ps {
			out = append(out, TopicPartition{Topic: t, Partition: p})
		}
	}
	return out
}
