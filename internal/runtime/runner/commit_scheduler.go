package runner

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/delivery"
)

// Committer commits source positions (the delivery strategy).
type Committer interface {
	Commit(ctx context.Context, marks []delivery.Mark) error
}

// CommitScheduler commits advanced marks every interval and on revoke, swap and stop.
type CommitScheduler struct {
	tracker   *OffsetTracker
	committer Committer
	interval  time.Duration
	log       *slog.Logger
	mu        sync.Mutex
	committed map[TopicPartition]int64
}

// NewCommitScheduler builds the scheduler.
func NewCommitScheduler(tracker *OffsetTracker, committer Committer, interval time.Duration, log *slog.Logger) *CommitScheduler {
	return &CommitScheduler{tracker: tracker, committer: committer, interval: interval, log: log, committed: map[TopicPartition]int64{}}
}

// Run commits on every tick until ctx ends.
func (c *CommitScheduler) Run(ctx context.Context) {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := c.CommitNow(ctx, nil); err != nil {
				c.log.Warn("runtime: commit failed; marks kept", string(logging.AttrError), err)
			}
		}
	}
}

// CommitNow commits the advanced marks of tps (nil = every partition); failures keep the marks for later.
func (c *CommitScheduler) CommitNow(ctx context.Context, tps []TopicPartition) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	marks := c.tracker.Marks()
	if tps != nil {
		marks = c.tracker.MarksFor(tps)
	}
	pending := []delivery.Mark{}
	for _, m := range marks {
		if prev, ok := c.committed[m.TopicPartition]; !ok || m.Offset > prev {
			pending = append(pending, delivery.Mark{Topic: m.Topic, Partition: m.Partition, Offset: m.Offset})
		}
	}
	if len(pending) == 0 {
		return nil
	}
	if err := c.committer.Commit(ctx, pending); err != nil {
		return err
	}
	for _, m := range pending {
		c.committed[TopicPartition{Topic: m.Topic, Partition: m.Partition}] = m.Offset
	}
	return nil
}

// Committed returns the last committed position of tp.
func (c *CommitScheduler) Committed(tp TopicPartition) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	off, ok := c.committed[tp]
	return off, ok
}

// Forget drops committed positions of revoked partitions.
func (c *CommitScheduler) Forget(tps []TopicPartition) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, tp := range tps {
		delete(c.committed, tp)
	}
}
