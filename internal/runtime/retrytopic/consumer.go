package retrytopic

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/retry"
)

// Handler finishes one due tier record: reprocess and deliver, schedule the next attempt or park it.
// nil means the record is final and its tier offset is committed; an error retries it after backoff.
type Handler interface {
	Handle(ctx context.Context, env Envelope) error
}

// Committer commits tier offsets of the retry consumer group (an at-least-once delivery strategy).
type Committer interface {
	Commit(ctx context.Context, marks []delivery.Mark) error
}

// LaneLimit bounds the records buffered per tier partition before its fetching is paused.
type LaneLimit int

const (
	LaneHighWater LaneLimit = 64
	LaneLowWater  LaneLimit = 16
	PollRecords   LaneLimit = 256
)

// ConsumerTiming holds the consumer's waits.
type ConsumerTiming time.Duration

const LaneStopWait ConsumerTiming = ConsumerTiming(10 * time.Second)

// Consumer reads every tier topic of a runner in its own group; each tier partition runs on its own lane, so a
// record that is not due yet only holds back its own tier partition, never the source or other tiers.
type Consumer struct {
	Handler   Handler
	Committer Committer
	Backoff   retry.Policy
	Now       func() time.Time
	Metrics   *Metrics
	RouteID   string
	Log       *slog.Logger

	mu     sync.Mutex
	client *kgo.Client
	lanes  map[TierPartition]*tierLane
}

// TierPartition identifies one partition of one tier topic.
type TierPartition struct {
	Topic     string
	Partition int32
}

// Options returns the group callbacks the consumer's client must be opened with.
func (c *Consumer) Options() []kgo.Opt {
	stop := func(_ context.Context, _ *kgo.Client, m map[string][]int32) { c.stopLanes(m) }
	return []kgo.Opt{kgo.DisableAutoCommit(), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.OnPartitionsRevoked(stop), kgo.OnPartitionsLost(stop)}
}

// Run polls client until ctx ends, then stops every lane.
func (c *Consumer) Run(ctx context.Context, client *kgo.Client) {
	c.mu.Lock()
	c.client, c.lanes = client, map[TierPartition]*tierLane{}
	c.mu.Unlock()
	defer c.stopLanes(nil)
	for {
		fetches := client.PollRecords(ctx, int(PollRecords))
		if ctx.Err() != nil || fetches.IsClientClosed() {
			return
		}
		fetches.EachPartition(func(p kgo.FetchTopicPartition) { c.dispatch(ctx, p) })
		c.Metrics.Pending(c.RouteID, c.pending())
	}
}

// dispatch hands polled records to the lane of their tier partition.
func (c *Consumer) dispatch(ctx context.Context, p kgo.FetchTopicPartition) {
	if p.Err != nil {
		c.Log.Debug("runtime: retry tier fetch error", string(logging.AttrRouteID), c.RouteID, string(logging.AttrError), p.Err)
	}
	l := c.laneFor(ctx, TierPartition{Topic: p.Topic, Partition: p.Partition})
	l.hw.Store(p.HighWatermark)
	if len(p.Records) > 0 {
		l.enqueue(p.Records)
	}
}

// laneFor returns the lane of tp, starting it on first use.
func (c *Consumer) laneFor(ctx context.Context, tp TierPartition) *tierLane {
	c.mu.Lock()
	defer c.mu.Unlock()
	if l, ok := c.lanes[tp]; ok {
		return l
	}
	l := newTierLane(ctx, tp, c)
	c.lanes[tp] = l
	go l.run()
	return l
}

// stopLanes stops the lanes of m (nil = all) and waits for their current record (bounded).
func (c *Consumer) stopLanes(m map[string][]int32) {
	c.mu.Lock()
	stopped := []*tierLane{}
	for tp, l := range c.lanes {
		if m == nil || TierPartitions(m).Has(tp) {
			stopped = append(stopped, l)
			delete(c.lanes, tp)
		}
	}
	c.mu.Unlock()
	deadline := time.After(time.Duration(LaneStopWait))
	for _, l := range stopped {
		l.stop()
		select {
		case <-l.done:
		case <-deadline:
		}
	}
}

// pending sums the tier records not yet finished over every lane.
func (c *Consumer) pending() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	var n int64
	for _, l := range c.lanes {
		n += l.pending()
	}
	return n
}

// pause stops fetching of tp (its lane is full).
func (c *Consumer) pause(tp TierPartition) {
	c.client.PauseFetchPartitions(map[string][]int32{tp.Topic: {tp.Partition}})
}

// resume restarts fetching of tp.
func (c *Consumer) resume(tp TierPartition) {
	c.client.ResumeFetchPartitions(map[string][]int32{tp.Topic: {tp.Partition}})
}

// TierPartitions is a franz-go topic → partitions map.
type TierPartitions map[string][]int32

// Has reports whether tp is in the map.
func (m TierPartitions) Has(tp TierPartition) bool {
	for _, p := range m[tp.Topic] {
		if p == tp.Partition {
			return true
		}
	}
	return false
}
