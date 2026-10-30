package retrytopic

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/delivery"
)

// tierLane processes the records of one tier partition in offset order: wait until due, handle, commit.
type tierLane struct {
	tp      TierPartition
	c       *Consumer
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	cond    *sync.Cond
	queue   []*kgo.Record
	stopped bool
	paused  bool
	hw      atomic.Int64
	next    atomic.Int64
	done    chan struct{}
}

func newTierLane(parent context.Context, tp TierPartition, c *Consumer) *tierLane {
	ctx, cancel := context.WithCancel(parent)
	l := &tierLane{tp: tp, c: c, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	l.cond = sync.NewCond(&l.mu)
	l.next.Store(-1)
	return l
}

// enqueue appends polled records; a full lane pauses fetching of its partition.
func (l *tierLane) enqueue(recs []*kgo.Record) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped {
		return
	}
	if l.next.Load() < 0 {
		l.next.Store(recs[0].Offset)
	}
	l.queue = append(l.queue, recs...)
	if !l.paused && len(l.queue) >= int(LaneHighWater) {
		l.paused = true
		l.c.pause(l.tp)
	}
	l.cond.Broadcast()
}

// run handles records until the lane stops.
func (l *tierLane) run() {
	defer close(l.done)
	for {
		rec, ok := l.take()
		if !ok {
			return
		}
		if !l.finish(rec) {
			return
		}
	}
}

// take blocks for the next record and resumes fetching once the lane is short again.
func (l *tierLane) take() (*kgo.Record, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for len(l.queue) == 0 && !l.stopped {
		l.cond.Wait()
	}
	if l.stopped {
		return nil, false
	}
	rec := l.queue[0]
	l.queue = l.queue[1:]
	if l.paused && len(l.queue) <= int(LaneLowWater) {
		l.paused = false
		l.c.resume(l.tp)
	}
	return rec, true
}

// finish waits until rec is due, handles it until final and commits; false when the lane stopped first.
func (l *tierLane) finish(rec *kgo.Record) bool {
	env, err := EnvelopeCodec{}.Decode(delivery.RecordMapper{}.FromKafka(rec))
	if err != nil {
		l.c.Log.Warn("runtime: unreadable retry tier record skipped", string(logging.AttrRouteID), l.c.RouteID, string(logging.AttrError), err)
		return l.commit(rec)
	}
	if !l.waitUntil(env.DueAt) {
		return false
	}
	for attempt := 0; ; attempt++ {
		if l.c.Handler.Handle(l.ctx, env) == nil {
			return l.commit(rec)
		}
		if l.c.Backoff.Wait(l.ctx, attempt) != nil {
			return false
		}
	}
}

// waitUntil sleeps until due or the lane stops.
func (l *tierLane) waitUntil(due time.Time) bool {
	wait := due.Sub(l.c.Now())
	if wait <= 0 {
		return l.ctx.Err() == nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-l.ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// commit stores the next tier offset of the group; a failed commit only means the record may run again.
func (l *tierLane) commit(rec *kgo.Record) bool {
	mark := delivery.Mark{Topic: rec.Topic, Partition: rec.Partition, Offset: rec.Offset + 1}
	if err := l.c.Committer.Commit(l.ctx, []delivery.Mark{mark}); err != nil {
		l.c.Log.Warn("runtime: retry tier commit failed", string(logging.AttrRouteID), l.c.RouteID, string(logging.AttrError), err)
	}
	l.next.Store(mark.Offset)
	return l.ctx.Err() == nil
}

// pending is the lane's records not yet finished (high watermark − next offset).
func (l *tierLane) pending() int64 {
	next := l.next.Load()
	if next < 0 {
		return 0
	}
	return max(l.hw.Load()-next, 0)
}

// stop ends the lane: queued records are dropped (uncommitted, re-read by the next owner).
func (l *tierLane) stop() {
	l.mu.Lock()
	l.stopped = true
	l.queue = nil
	l.cond.Broadcast()
	l.mu.Unlock()
	l.cancel()
}
