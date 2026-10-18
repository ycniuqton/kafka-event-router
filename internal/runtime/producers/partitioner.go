package producers

import (
	"context"
	"sync/atomic"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/pkg/config"
)

// modeKey carries the destination partitioning mode on a record's Context.
type modeKey struct{}

// PartitionModes tags produce contexts with destination partitioning modes.
type PartitionModes struct{}

// Tag returns ctx carrying the destination partitioning mode of the record produced under it.
func (PartitionModes) Tag(ctx context.Context, mode config.PartitioningMode) context.Context {
	return context.WithValue(ctx, modeKey{}, mode)
}

// PartitionStrategy is the base of every destination partitioning mode.
type PartitionStrategy interface {
	Mode() config.PartitioningMode
	Partition(r *kgo.Record, n int) int
}

// ByKeyStrategy hashes keys like Kafka (murmur2); keyless records are spread round robin.
type ByKeyStrategy struct {
	hashed kgo.TopicPartitioner
	spread *RoundRobinStrategy
}

func (ByKeyStrategy) Mode() config.PartitioningMode { return config.PartitioningModeByKey }

func (s ByKeyStrategy) Partition(r *kgo.Record, n int) int {
	if r.Key == nil {
		return s.spread.Partition(r, n)
	}
	return s.hashed.Partition(r, n)
}

// RoundRobinStrategy cycles through partitions.
type RoundRobinStrategy struct {
	next atomic.Uint64
}

func (*RoundRobinStrategy) Mode() config.PartitioningMode { return config.PartitioningModeRoundRobin }

func (s *RoundRobinStrategy) Partition(_ *kgo.Record, n int) int {
	return int((s.next.Add(1) - 1) % uint64(n))
}

// SameAsSourceStrategy keeps the source partition number (modulo the destination count).
type SameAsSourceStrategy struct{}

func (SameAsSourceStrategy) Mode() config.PartitioningMode {
	return config.PartitioningModeSameAsSource
}

func (SameAsSourceStrategy) Partition(r *kgo.Record, n int) int {
	return int(r.Partition) % n
}

// DestinationPartitioner is the producer partitioner honouring each record's destination mode.
type DestinationPartitioner struct {
	Default config.PartitioningMode
}

// ForTopic implements kgo.Partitioner.
func (p DestinationPartitioner) ForTopic(topic string) kgo.TopicPartitioner {
	spread := &RoundRobinStrategy{}
	strategies := map[config.PartitioningMode]PartitionStrategy{}
	for _, s := range []PartitionStrategy{ByKeyStrategy{hashed: kgo.StickyKeyPartitioner(nil).ForTopic(topic), spread: spread}, spread, SameAsSourceStrategy{}} {
		strategies[s.Mode()] = s
	}
	return &destinationTopicPartitioner{strategies: strategies, fallback: p.Default}
}

// destinationTopicPartitioner dispatches each record to its mode's strategy.
type destinationTopicPartitioner struct {
	strategies map[config.PartitioningMode]PartitionStrategy
	fallback   config.PartitioningMode
}

func (*destinationTopicPartitioner) RequiresConsistency(*kgo.Record) bool { return true }

func (d *destinationTopicPartitioner) Partition(r *kgo.Record, n int) int {
	mode := d.fallback
	if r.Context != nil {
		if m, ok := r.Context.Value(modeKey{}).(config.PartitioningMode); ok {
			mode = m
		}
	}
	s, ok := d.strategies[mode]
	if !ok {
		s = d.strategies[d.fallback]
	}
	return s.Partition(r, n)
}
