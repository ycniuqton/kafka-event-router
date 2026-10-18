package dlq

import (
	"context"
	"errors"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// FranzSource implements PartitionSource with kadm list-offsets and direct partition consumers.
type FranzSource struct {
	Clients ClientOpener
}

// Ranges implements PartitionSource.
func (s FranzSource) Ranges(ctx context.Context, t Target, since, until time.Time) (RangeSet, error) {
	cl, err := s.Clients.Open(ctx, t.ConnectionID, kafka.PurposeAdmin)
	if err != nil {
		return RangeSet{}, err
	}
	defer cl.Close()
	lookup := OffsetLookup{Admin: kadm.NewClient(cl), Topics: t.Topics}
	set, err := lookup.Watermarks(ctx)
	if err != nil {
		return RangeSet{}, ErrUpstream.Wrap(err)
	}
	if err := lookup.Narrow(ctx, set.Ranges, since, until); err != nil {
		return RangeSet{}, ErrUpstream.Wrap(err)
	}
	return set, nil
}

// Read implements PartitionSource: one direct consumer for all windows, closed afterwards.
func (s FranzSource) Read(ctx context.Context, connectionID string, windows []kafka.PartitionWindow) ([]engine.RawRecord, error) {
	plan := NewWindowPlan(windows)
	if plan.Empty() {
		return nil, nil
	}
	cl, err := s.Clients.Open(ctx, connectionID, kafka.PurposeSampler, kgo.ConsumePartitions(plan.Offsets()))
	if err != nil {
		return nil, err
	}
	defer cl.Close()
	return WindowPoller{Client: cl, Plan: plan}.Poll(ctx)
}

// OffsetLookup lists watermarks and time offsets of a set of topics.
type OffsetLookup struct {
	Admin  *kadm.Client
	Topics []string
}

// Watermarks returns low/high per partition; unknown topics are reported as missing.
func (l OffsetLookup) Watermarks(ctx context.Context) (RangeSet, error) {
	starts, err := l.Admin.ListStartOffsets(ctx, l.Topics...)
	if err != nil {
		return RangeSet{}, err
	}
	ends, err := l.Admin.ListEndOffsets(ctx, l.Topics...)
	if err != nil {
		return RangeSet{}, err
	}
	set := RangeSet{}
	for _, topic := range l.Topics {
		if l.missing(starts, topic) {
			set.Missing = append(set.Missing, topic)
			continue
		}
		set.Ranges = append(set.Ranges, l.topicRanges(starts, ends, topic)...)
	}
	return set, nil
}

func (l OffsetLookup) missing(listed kadm.ListedOffsets, topic string) bool {
	parts, ok := listed[topic]
	if !ok {
		return true
	}
	bad, ok := parts[-1]
	return ok && errors.Is(bad.Err, kerr.UnknownTopicOrPartition)
}

func (l OffsetLookup) topicRanges(starts, ends kadm.ListedOffsets, topic string) []PartitionRange {
	out := []PartitionRange{}
	for p, start := range starts[topic] {
		end, ok := ends.Lookup(topic, p)
		if !ok || start.Err != nil || end.Err != nil {
			continue
		}
		out = append(out, PartitionRange{Topic: topic, Partition: p, Low: start.Offset, High: end.Offset, From: start.Offset, To: end.Offset})
	}
	return out
}

// Narrow sets From/To from ListOffsets by timestamp (first record at/after since, first after until).
func (l OffsetLookup) Narrow(ctx context.Context, ranges []PartitionRange, since, until time.Time) error {
	if !since.IsZero() {
		listed, err := l.Admin.ListOffsetsAfterMilli(ctx, since.UnixMilli(), l.Topics...)
		if err != nil {
			return err
		}
		l.apply(ranges, listed, func(r *PartitionRange, off int64) { r.From = off })
	}
	if !until.IsZero() {
		listed, err := l.Admin.ListOffsetsAfterMilli(ctx, until.Add(time.Millisecond).UnixMilli(), l.Topics...)
		if err != nil {
			return err
		}
		l.apply(ranges, listed, func(r *PartitionRange, off int64) { r.To = off })
	}
	return nil
}

func (l OffsetLookup) apply(ranges []PartitionRange, listed kadm.ListedOffsets, set func(*PartitionRange, int64)) {
	for i := range ranges {
		o, ok := listed.Lookup(ranges[i].Topic, ranges[i].Partition)
		if ok && o.Err == nil && o.Offset >= 0 {
			set(&ranges[i], o.Offset)
		}
	}
}

// WindowPlan tracks which windows of a read are still open.
type WindowPlan struct {
	open map[PartitionKey]kafka.PartitionWindow
}

// NewWindowPlan keeps the non-empty windows.
func NewWindowPlan(windows []kafka.PartitionWindow) *WindowPlan {
	p := &WindowPlan{open: map[PartitionKey]kafka.PartitionWindow{}}
	for _, w := range windows {
		if w.Len() > 0 {
			p.open[KeyOf(w.Topic, w.Partition)] = w
		}
	}
	return p
}

// Empty reports whether every window is closed.
func (p *WindowPlan) Empty() bool { return len(p.open) == 0 }

// Offsets returns the start offsets to assign.
func (p *WindowPlan) Offsets() map[string]map[int32]kgo.Offset {
	out := map[string]map[int32]kgo.Offset{}
	for _, w := range p.open {
		if out[w.Topic] == nil {
			out[w.Topic] = map[int32]kgo.Offset{}
		}
		out[w.Topic][w.Partition] = kgo.NewOffset().At(w.Start)
	}
	return out
}

// Accept reports whether r lies in its open window and closes the window at its last offset.
func (p *WindowPlan) Accept(r *kgo.Record) bool {
	k := KeyOf(r.Topic, r.Partition)
	w, ok := p.open[k]
	if !ok {
		return false
	}
	if r.Offset >= w.End-1 {
		delete(p.open, k)
	}
	return r.Offset >= w.Start && r.Offset < w.End
}

// WindowPoller polls a direct consumer until every window closed or the partition went idle.
type WindowPoller struct {
	Client *kgo.Client
	Plan   *WindowPlan
}

// Poll returns the records inside the windows; an idle poll ends the read (gaps, markers).
func (p WindowPoller) Poll(ctx context.Context) ([]engine.RawRecord, error) {
	out := []engine.RawRecord{}
	for !p.Plan.Empty() {
		pollCtx, cancel := context.WithTimeout(ctx, IdlePoll.Duration())
		fetches := p.Client.PollFetches(pollCtx)
		cancel()
		if err := p.fetchError(ctx, fetches); err != nil {
			return out, err
		}
		if fetches.NumRecords() == 0 {
			return out, ctx.Err()
		}
		fetches.EachRecord(func(r *kgo.Record) {
			if p.Plan.Accept(r) {
				out = append(out, RawOf{}.Record(r))
			}
		})
	}
	return out, nil
}

func (p WindowPoller) fetchError(ctx context.Context, fetches kgo.Fetches) error {
	for _, fe := range fetches.Errors() {
		if errors.Is(fe.Err, context.DeadlineExceeded) || errors.Is(fe.Err, context.Canceled) {
			continue
		}
		return ErrUpstream.Wrap(fe.Err)
	}
	return ctx.Err()
}

// RawOf converts franz-go records to engine raw records.
type RawOf struct{}

// Record copies key, value, headers and coordinates.
func (RawOf) Record(r *kgo.Record) engine.RawRecord {
	hs := make([]engine.Header, 0, len(r.Headers))
	for _, h := range r.Headers {
		hs = append(hs, engine.Header{Key: h.Key, Value: h.Value})
	}
	return engine.RawRecord{Key: r.Key, Value: r.Value, Headers: hs, Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp}
}
