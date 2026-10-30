package runner

import (
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
)

// pauseRecorder counts pause and resume calls of the gate.
type pauseRecorder struct {
	mu      sync.Mutex
	pauses  int
	resumes int
}

func (p *pauseRecorder) PauseFetchPartitions(map[string][]int32) map[string][]int32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pauses++
	return nil
}

func (p *pauseRecorder) ResumeFetchPartitions(map[string][]int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resumes++
}

// laneFixture builds a lane over a recording gate and record batches of one size.
type laneFixture struct {
	pauser *pauseRecorder
	lane   *PartitionLane
}

func newLaneFixture() laneFixture {
	p := &pauseRecorder{}
	return laneFixture{pauser: p, lane: NewPartitionLane(TopicPartition{Topic: "in"}, NewBackpressureGate(p))}
}

func (laneFixture) batch(n, size int) []*kgo.Record {
	out := make([]*kgo.Record, n)
	for i := range out {
		out[i] = &kgo.Record{Value: make([]byte, size)}
	}
	return out
}

func TestLaneOnePollNeverPauses(t *testing.T) {
	f := newLaneFixture()
	f.lane.Enqueue(f.batch(int(MaxPollRecords), 1024))
	if f.pauser.pauses != 0 || f.lane.gate.Paused(f.lane.TP()) {
		t.Fatalf("one poll of %d 1 KB records paused the lane", MaxPollRecords)
	}
}

func TestLanePausesOnRecordsAndResumesBelowLowWater(t *testing.T) {
	f := newLaneFixture()
	f.lane.Enqueue(f.batch(int(LaneHighWater), 16))
	if f.pauser.pauses != 1 {
		t.Fatalf("pauses = %d at the record high-water mark, want 1", f.pauser.pauses)
	}
	for i := 0; i < int(LaneHighWater-LaneLowWater); i++ {
		if _, ok := f.lane.next(); !ok {
			t.Fatal("lane stopped")
		}
	}
	if f.pauser.resumes != 1 || f.lane.gate.Paused(f.lane.TP()) {
		t.Fatalf("resumes = %d at the low-water mark, want 1", f.pauser.resumes)
	}
}

func TestLanePausesOnBytesBeforeRecordMark(t *testing.T) {
	f := newLaneFixture()
	size := 100 << 10
	n := int(int64(LaneHighWaterBytes)/int64(size)) + 1
	f.lane.Enqueue(f.batch(n, size))
	if f.pauser.pauses != 1 {
		t.Fatalf("%d records of 100 KB did not pause the lane", n)
	}
	for f.lane.gate.Paused(f.lane.TP()) {
		if _, ok := f.lane.next(); !ok {
			t.Fatal("lane stopped")
		}
	}
	if f.lane.QueuedBytes() > int64(LaneLowWaterBytes) {
		t.Fatalf("resumed with %d bytes queued, above the low-water mark", f.lane.QueuedBytes())
	}
}

func TestLaneDoesNotKeepHandedOutRecordsAlive(t *testing.T) {
	f := newLaneFixture()
	f.lane.Enqueue(f.batch(4, 16))
	backing := f.lane.queue[:4]
	for i := 0; i < 3; i++ {
		if _, ok := f.lane.next(); !ok {
			t.Fatal("lane stopped")
		}
	}
	for i := 0; i < 3; i++ {
		if backing[i] != nil {
			t.Fatalf("slot %d of the queue's backing array still references a handed-out record", i)
		}
	}
	if backing[3] == nil {
		t.Fatal("the queued record must stay")
	}
}
