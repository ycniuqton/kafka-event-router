package kafka

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// FranzMarksReader implements PartitionMarksReader with kadm ListOffsets (start, end, after time).
// Unknown topics are left out of the result so callers can report them by name.
type FranzMarksReader struct {
	adm *kadm.Client
}

// NewFranzMarksReader wraps a franz-go client (the caller keeps ownership).
func NewFranzMarksReader(cl *kgo.Client) *FranzMarksReader {
	return &FranzMarksReader{adm: kadm.NewClient(cl)}
}

// Marks implements PartitionMarksReader; a zero from leaves FromOffset unresolved.
func (r *FranzMarksReader) Marks(ctx context.Context, topics []string, from time.Time) ([]PartitionMarks, error) {
	starts, err := r.adm.ListStartOffsets(ctx, topics...)
	if err != nil {
		return nil, err
	}
	ends, err := r.adm.ListEndOffsets(ctx, topics...)
	if err != nil {
		return nil, err
	}
	after, err := r.after(ctx, topics, from)
	if err != nil {
		return nil, err
	}
	return ListedMarks{Starts: starts, Ends: ends, After: after}.Build()
}

// after lists the first offsets at or after from, or nothing when no start time was asked.
func (r *FranzMarksReader) after(ctx context.Context, topics []string, from time.Time) (kadm.ListedOffsets, error) {
	if from.IsZero() {
		return kadm.ListedOffsets{}, nil
	}
	return r.adm.ListOffsetsAfterMilli(ctx, from.UnixMilli(), topics...)
}

// ListedMarks joins the three ListOffsets answers into one PartitionMarks per partition.
type ListedMarks struct {
	Starts kadm.ListedOffsets
	Ends   kadm.ListedOffsets
	After  kadm.ListedOffsets
}

// Build returns the marks in topic/partition order; the first real partition error fails the call.
func (l ListedMarks) Build() ([]PartitionMarks, error) {
	out := []PartitionMarks{}
	var firstErr error
	l.Ends.Each(func(end kadm.ListedOffset) {
		if err := (ListedOffsetCheck{}).Err(end, l.Starts); err != nil {
			firstErr = errors.Join(firstErr, err)
			return
		}
		if end.Err != nil {
			return
		}
		start, _ := l.Starts.Lookup(end.Topic, end.Partition)
		out = append(out, PartitionMarks{Topic: end.Topic, Partition: end.Partition, Low: start.Offset, High: end.Offset, FromOffset: l.fromOffset(end)})
	})
	if firstErr != nil {
		return nil, firstErr
	}
	return SamplingMarksOrder{}.Sort(out), nil
}

func (l ListedMarks) fromOffset(end kadm.ListedOffset) int64 {
	o, ok := l.After.Lookup(end.Topic, end.Partition)
	if !ok || o.Err != nil || o.Offset < 0 {
		return int64(OffsetUnresolved)
	}
	return o.Offset
}

// ListedOffsetCheck separates missing topics (skipped) from real listing errors.
type ListedOffsetCheck struct{}

// Err returns the error to report for a partition, nil for a healthy or unknown-topic entry.
func (ListedOffsetCheck) Err(end kadm.ListedOffset, starts kadm.ListedOffsets) error {
	if end.Err != nil && !errors.Is(end.Err, kerr.UnknownTopicOrPartition) {
		return end.Err
	}
	start, ok := starts.Lookup(end.Topic, end.Partition)
	if end.Err == nil && ok && start.Err != nil {
		return start.Err
	}
	return nil
}

// SamplingMarksOrder gives marks a stable topic/partition order.
type SamplingMarksOrder struct{}

// Sort orders marks by topic then partition.
func (SamplingMarksOrder) Sort(ms []PartitionMarks) []PartitionMarks {
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].Topic != ms[j].Topic {
			return ms[i].Topic < ms[j].Topic
		}
		return ms[i].Partition < ms[j].Partition
	})
	return ms
}

// PartitionKey identifies one partition of one topic.
type PartitionKey struct {
	Topic     string
	Partition int32
}
