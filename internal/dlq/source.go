package dlq

import (
	"context"
	"time"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// PartitionRange is one DLQ partition: watermarks plus the offsets matching a time range.
// From is the first offset at/after Since (Low when unset); To the first offset after Until (High when unset).
type PartitionRange struct {
	Topic     string
	Partition int32
	Low       int64
	High      int64
	From      int64
	To        int64
}

// Key returns the partition key.
func (r PartitionRange) Key() PartitionKey { return KeyOf(r.Topic, r.Partition) }

// Lower returns the first offset to scan.
func (r PartitionRange) Lower() int64 { return max(r.Low, r.From) }

// RangeSet is the result of a marks lookup: ranges of existing topics and the missing topics.
type RangeSet struct {
	Ranges  []PartitionRange
	Missing []string
}

// PartitionSource is the Kafka I/O port of the reader: watermarks/time offsets and window reads,
// always with manual assignment and no consumer group.
type PartitionSource interface {
	Ranges(ctx context.Context, t Target, since, until time.Time) (RangeSet, error)
	Read(ctx context.Context, connectionID string, windows []kafka.PartitionWindow) ([]engine.RawRecord, error)
}
