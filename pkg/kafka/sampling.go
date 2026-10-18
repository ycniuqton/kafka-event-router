package kafka

import (
	"context"
	"time"
)

// StopReason tells why a sampling read ended.
type StopReason string

const (
	StopNone      StopReason = ""
	StopComplete  StopReason = "complete"
	StopMaxEvents StopReason = "max_events"
	StopMaxBytes  StopReason = "max_bytes"
	StopDeadline  StopReason = "deadline"
)

// Truncated reports whether the read ended before every window was fully read.
func (r StopReason) Truncated() bool {
	return r != StopNone && r != StopComplete
}

// SamplingOffset names special offset values used by sampling plans.
type SamplingOffset int64

const OffsetUnresolved SamplingOffset = -1

// PartitionWindow is the half-open offset range [Start, End) to read from one partition;
// End is the high-watermark snapshot, Until (optional) stops at records newer than it.
type PartitionWindow struct {
	Topic     string
	Partition int32
	Start     int64
	End       int64
	Until     time.Time
}

// Len returns the number of offsets in the window.
func (w PartitionWindow) Len() int64 {
	if w.End <= w.Start {
		return 0
	}
	return w.End - w.Start
}

// PartitionMarks are the watermarks of one partition plus the offset of the first record
// at or after the requested start time (OffsetUnresolved when there is none or no time was asked).
type PartitionMarks struct {
	Topic      string
	Partition  int32
	Low        int64
	High       int64
	FromOffset int64
}

// Available returns how many records the partition currently holds.
func (m PartitionMarks) Available() int64 {
	if m.High <= m.Low {
		return 0
	}
	return m.High - m.Low
}

// SamplingPlan is what the caller wants: the latest Count records, or the records in [From, To].
type SamplingPlan struct {
	Count int
	From  time.Time
	To    time.Time
}

// SampledHeader is one Kafka header of a sampled record.
type SampledHeader struct {
	Key   string
	Value []byte
}

// SampledRecord is one sampled Kafka record.
type SampledRecord struct {
	Topic     string
	Partition int32
	Offset    int64
	Timestamp time.Time
	Key       []byte
	Value     []byte
	Headers   []SampledHeader
}

// Size is the byte size counted against the sampling budget (key + value + headers).
func (r SampledRecord) Size() int64 {
	size := int64(len(r.Key) + len(r.Value))
	for _, h := range r.Headers {
		size += int64(len(h.Key) + len(h.Value))
	}
	return size
}

// PartitionMarksReader lists partitions of topics with watermarks, optionally resolving
// the first offset at or after from (ListOffsets by timestamp).
type PartitionMarksReader interface {
	Marks(ctx context.Context, topics []string, from time.Time) ([]PartitionMarks, error)
}

// PartitionSampler reads windows with manual partition assignment: no group id, no commits.
type PartitionSampler interface {
	Read(ctx context.Context, windows []PartitionWindow, budget *SamplingBudget) ([]SampledRecord, StopReason, error)
}
