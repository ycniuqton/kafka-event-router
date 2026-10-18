package runner

import "sync"

// TopicPartition identifies one source partition.
type TopicPartition struct {
	Topic     string
	Partition int32
}

// PartitionMark is the next offset to consume for one partition (the committable position).
type PartitionMark struct {
	TopicPartition
	Offset int64
}

// trackedOffset is one record whose outcome is still pending or already decided.
type trackedOffset struct {
	offset int64
	done   bool
}

// partitionOffsets keeps the ordered tracked offsets of one partition and its contiguous mark.
type partitionOffsets struct {
	queue   []*trackedOffset
	byOff   map[int64]*trackedOffset
	mark    int64
	hasMark bool
}

// advance pops finished offsets from the head and moves the committable mark past them.
func (p *partitionOffsets) advance() {
	for len(p.queue) > 0 && p.queue[0].done {
		head := p.queue[0]
		p.queue = p.queue[1:]
		delete(p.byOff, head.offset)
		p.mark, p.hasMark = head.offset+1, true
	}
}

// OffsetTracker advances, per partition, the highest offset whose record and every earlier record is
// done (produced, filtered, dead-lettered, skipped); offsets are never committed past an unfinished one.
type OffsetTracker struct {
	mu    sync.Mutex
	parts map[TopicPartition]*partitionOffsets
}

// NewOffsetTracker returns an empty tracker.
func NewOffsetTracker() *OffsetTracker {
	return &OffsetTracker{parts: map[TopicPartition]*partitionOffsets{}}
}

// Track registers a record read in partition order.
func (t *OffsetTracker) Track(tp TopicPartition, offset int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.parts[tp]
	if !ok {
		p = &partitionOffsets{byOff: map[int64]*trackedOffset{}}
		t.parts[tp] = p
	}
	e := &trackedOffset{offset: offset}
	p.queue = append(p.queue, e)
	p.byOff[offset] = e
}

// MarkDone records the outcome of a tracked record and advances the contiguous mark.
func (t *OffsetTracker) MarkDone(tp TopicPartition, offset int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.parts[tp]
	if !ok {
		return
	}
	if e, ok := p.byOff[offset]; ok {
		e.done = true
		p.advance()
	}
}

// Position returns the first offset of tp not yet done: the mark, or the oldest tracked offset before any
// record finished (false when nothing was tracked).
func (t *OffsetTracker) Position(tp TopicPartition) (int64, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.parts[tp]
	switch {
	case !ok:
		return 0, false
	case p.hasMark:
		return p.mark, true
	case len(p.queue) > 0:
		return p.queue[0].offset, true
	}
	return 0, false
}

// Marks returns the committable position of every partition that has one.
func (t *OffsetTracker) Marks() []PartitionMark {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]PartitionMark, 0, len(t.parts))
	for tp, p := range t.parts {
		if p.hasMark {
			out = append(out, PartitionMark{TopicPartition: tp, Offset: p.mark})
		}
	}
	return out
}

// MarksFor returns the committable positions of the given partitions only.
func (t *OffsetTracker) MarksFor(tps []TopicPartition) []PartitionMark {
	want := map[TopicPartition]bool{}
	for _, tp := range tps {
		want[tp] = true
	}
	out := []PartitionMark{}
	for _, m := range t.Marks() {
		if want[m.TopicPartition] {
			out = append(out, m)
		}
	}
	return out
}

// Pending returns how many tracked records of tp are not yet done or still wait behind one.
func (t *OffsetTracker) Pending(tp TopicPartition) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if p, ok := t.parts[tp]; ok {
		return len(p.queue)
	}
	return 0
}

// Forget drops the state of revoked or lost partitions.
func (t *OffsetTracker) Forget(tps []TopicPartition) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, tp := range tps {
		delete(t.parts, tp)
	}
}

// Tracked returns every partition with tracked state.
func (t *OffsetTracker) Tracked() []TopicPartition {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TopicPartition, 0, len(t.parts))
	for tp := range t.parts {
		out = append(out, tp)
	}
	return out
}
