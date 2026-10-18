package kafka

import (
	"context"
	"errors"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// SamplerTiming bounds the polling of one sampling read: IdleWait ends a started read when no record
// arrives (the rest of a window is transaction markers or compacted away), FirstWait bounds the first
// poll when the budget has no deadline.
type SamplerTiming time.Duration

const (
	SamplerIdleWait  SamplerTiming = SamplerTiming(3 * time.Second)
	SamplerFirstWait SamplerTiming = SamplerTiming(10 * time.Second)
	SamplerMinWait   SamplerTiming = SamplerTiming(time.Millisecond)
)

// FranzPartitionSampler implements PartitionSampler with manual partition assignment on a
// group-less franz-go client: no group id, no commits, stop at each window's HWM snapshot.
type FranzPartitionSampler struct {
	raw  *kgo.Client
	idle time.Duration
	now  func() time.Time
}

// NewFranzPartitionSampler wraps a franz-go client opened without a consumer group.
func NewFranzPartitionSampler(cl *kgo.Client) *FranzPartitionSampler {
	return &FranzPartitionSampler{raw: cl, idle: time.Duration(SamplerIdleWait), now: time.Now}
}

// WithIdleWait returns a copy using another idle wait (tests).
func (s *FranzPartitionSampler) WithIdleWait(d time.Duration) *FranzPartitionSampler {
	c := *s
	c.idle = d
	return &c
}

// Read implements PartitionSampler; partitions are unassigned again before it returns.
func (s *FranzPartitionSampler) Read(ctx context.Context, windows []PartitionWindow, budget *SamplingBudget) ([]SampledRecord, StopReason, error) {
	progress := NewWindowProgress(windows)
	if progress.Done() {
		return []SampledRecord{}, budget.Reason(), nil
	}
	s.raw.AddConsumePartitions(progress.Assignments())
	defer s.raw.RemoveConsumePartitions(progress.Partitions())
	return s.drain(ctx, progress, budget)
}

// drain polls until every window is finished, the budget stops the read, or the partitions go idle.
func (s *FranzPartitionSampler) drain(ctx context.Context, progress *WindowProgress, budget *SamplingBudget) ([]SampledRecord, StopReason, error) {
	out := []SampledRecord{}
	for !progress.Done() {
		if budget.Expired(s.now()) {
			return out, budget.Reason(), nil
		}
		fetches, err := s.poll(ctx, progress, budget)
		if err != nil {
			return out, StopNone, err
		}
		if fetches.NumRecords() == 0 {
			break
		}
		if !s.collect(fetches, progress, budget, &out) {
			break
		}
	}
	budget.Expired(s.now())
	return out, budget.Reason(), nil
}

// poll waits for the next fetches; an expired poll wait yields empty fetches, not an error.
func (s *FranzPartitionSampler) poll(ctx context.Context, progress *WindowProgress, budget *SamplingBudget) (kgo.Fetches, error) {
	pollCtx, cancel := context.WithTimeout(ctx, s.wait(progress, budget))
	defer cancel()
	fetches := s.raw.PollFetches(pollCtx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return fetches, (FetchErrors{}).First(fetches)
}

// wait is the idle wait once records arrived, else the first wait, both capped by the budget deadline.
func (s *FranzPartitionSampler) wait(progress *WindowProgress, budget *SamplingBudget) time.Duration {
	wait := time.Duration(SamplerFirstWait)
	if progress.Started() {
		wait = s.idle
	}
	if left, bounded := budget.Remaining(s.now()); bounded {
		return max(min(wait, left), time.Duration(SamplerMinWait))
	}
	return wait
}

// collect admits the polled records of open windows; false means the budget refused a record.
func (s *FranzPartitionSampler) collect(fetches kgo.Fetches, progress *WindowProgress, budget *SamplingBudget, out *[]SampledRecord) bool {
	for it := fetches.RecordIter(); !it.Done(); {
		rec := it.Next()
		if !progress.Accepts(rec) {
			continue
		}
		sampled := SampledRecordOf(rec)
		if !budget.Admit(sampled.Size(), s.now()) {
			return false
		}
		*out = append(*out, sampled)
		progress.Advance(rec)
	}
	return true
}

// SampledRecordOf converts a franz-go record.
func SampledRecordOf(rec *kgo.Record) SampledRecord {
	headers := make([]SampledHeader, 0, len(rec.Headers))
	for _, h := range rec.Headers {
		headers = append(headers, SampledHeader{Key: h.Key, Value: h.Value})
	}
	return SampledRecord{Topic: rec.Topic, Partition: rec.Partition, Offset: rec.Offset, Timestamp: rec.Timestamp, Key: rec.Key, Value: rec.Value, Headers: headers}
}

// FetchErrors extracts the first real error of a poll (poll-wait expiry is not an error).
type FetchErrors struct{}

// First returns the first non-context fetch error.
func (FetchErrors) First(fetches kgo.Fetches) error {
	for _, fe := range fetches.Errors() {
		if errors.Is(fe.Err, context.DeadlineExceeded) || errors.Is(fe.Err, context.Canceled) {
			continue
		}
		return fe.Err
	}
	return nil
}

// WindowProgress tracks which windows of one read are still open.
type WindowProgress struct {
	windows  map[PartitionKey]PartitionWindow
	finished map[PartitionKey]bool
	started  bool
}

// NewWindowProgress opens every non-empty window.
func NewWindowProgress(windows []PartitionWindow) *WindowProgress {
	p := &WindowProgress{windows: map[PartitionKey]PartitionWindow{}, finished: map[PartitionKey]bool{}}
	for _, w := range windows {
		if w.Len() > 0 {
			p.windows[PartitionKey{Topic: w.Topic, Partition: w.Partition}] = w
		}
	}
	return p
}

// Assignments returns the start offset of every window for AddConsumePartitions.
func (p *WindowProgress) Assignments() map[string]map[int32]kgo.Offset {
	out := map[string]map[int32]kgo.Offset{}
	for k, w := range p.windows {
		if out[k.Topic] == nil {
			out[k.Topic] = map[int32]kgo.Offset{}
		}
		out[k.Topic][k.Partition] = kgo.NewOffset().At(w.Start)
	}
	return out
}

// Partitions returns the assigned partitions for RemoveConsumePartitions.
func (p *WindowProgress) Partitions() map[string][]int32 {
	out := map[string][]int32{}
	for k := range p.windows {
		out[k.Topic] = append(out[k.Topic], k.Partition)
	}
	return out
}

// Accepts reports whether rec belongs to an open window; records past its end close the window.
func (p *WindowProgress) Accepts(rec *kgo.Record) bool {
	k := PartitionKey{Topic: rec.Topic, Partition: rec.Partition}
	w, ok := p.windows[k]
	if !ok || p.finished[k] || rec.Offset < w.Start {
		return false
	}
	p.started = true
	if rec.Offset >= w.End || (!w.Until.IsZero() && rec.Timestamp.After(w.Until)) {
		p.finished[k] = true
		return false
	}
	return true
}

// Advance closes the window once its last offset was read.
func (p *WindowProgress) Advance(rec *kgo.Record) {
	k := PartitionKey{Topic: rec.Topic, Partition: rec.Partition}
	if rec.Offset >= p.windows[k].End-1 {
		p.finished[k] = true
	}
}

// Started reports whether any record of a window arrived.
func (p *WindowProgress) Started() bool { return p.started }

// Done reports whether every window is finished.
func (p *WindowProgress) Done() bool { return len(p.finished) >= len(p.windows) }

// FranzSamplingClient bundles a sampler-purpose client with its marks reader and sampler.
type FranzSamplingClient struct {
	client  Client
	marks   *FranzMarksReader
	sampler *FranzPartitionSampler
}

// NewFranzSamplingClient takes ownership of client (Close closes it).
func NewFranzSamplingClient(client Client) *FranzSamplingClient {
	raw := client.Raw()
	return &FranzSamplingClient{client: client, marks: NewFranzMarksReader(raw), sampler: NewFranzPartitionSampler(raw)}
}

// Marks implements PartitionMarksReader.
func (c *FranzSamplingClient) Marks(ctx context.Context, topics []string, from time.Time) ([]PartitionMarks, error) {
	return c.marks.Marks(ctx, topics, from)
}

// Read implements PartitionSampler.
func (c *FranzSamplingClient) Read(ctx context.Context, windows []PartitionWindow, budget *SamplingBudget) ([]SampledRecord, StopReason, error) {
	return c.sampler.Read(ctx, windows, budget)
}

// Close closes the underlying client.
func (c *FranzSamplingClient) Close() { c.client.Close() }
