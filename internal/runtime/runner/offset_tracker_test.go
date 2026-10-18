package runner

import (
	"math/rand/v2"
	"testing"
)

func TestOffsetTrackerNeverCommitsPastUnfinishedOffset(t *testing.T) {
	tp := TopicPartition{Topic: "in", Partition: 0}
	for seed := uint64(0); seed < 200; seed++ {
		rng := rand.New(rand.NewPCG(seed, seed*7+1))
		tr := NewOffsetTracker()
		offsets := []int64{}
		next := int64(rng.IntN(5))
		for i := 0; i < 50; i++ {
			next += int64(1 + rng.IntN(3))
			offsets = append(offsets, next)
			tr.Track(tp, next)
		}
		done := map[int64]bool{}
		for _, i := range rng.Perm(len(offsets)) {
			tr.MarkDone(tp, offsets[i])
			done[offsets[i]] = true
			assertMarkSafe(t, tr, tp, offsets, done)
		}
		marks := tr.Marks()
		if len(marks) != 1 || marks[0].Offset != offsets[len(offsets)-1]+1 {
			t.Fatalf("seed %d: final mark %+v, want %d", seed, marks, offsets[len(offsets)-1]+1)
		}
		if tr.Pending(tp) != 0 {
			t.Fatalf("seed %d: pending %d", seed, tr.Pending(tp))
		}
	}
}

func assertMarkSafe(t *testing.T, tr *OffsetTracker, tp TopicPartition, offsets []int64, done map[int64]bool) {
	t.Helper()
	marks := tr.MarksFor([]TopicPartition{tp})
	if len(marks) == 0 {
		if done[offsets[0]] {
			t.Fatal("first offset done but no mark")
		}
		return
	}
	for _, o := range offsets {
		if o < marks[0].Offset && !done[o] {
			t.Fatalf("mark %d passes unfinished offset %d", marks[0].Offset, o)
		}
	}
}

func TestOffsetTrackerForgetDropsPartition(t *testing.T) {
	tp := TopicPartition{Topic: "in", Partition: 1}
	tr := NewOffsetTracker()
	tr.Track(tp, 10)
	tr.MarkDone(tp, 10)
	tr.Forget([]TopicPartition{tp})
	if len(tr.Marks()) != 0 {
		t.Fatal("forgotten partition still has a mark")
	}
	tr.MarkDone(tp, 11)
}

func TestInFlightLimiterBounds(t *testing.T) {
	l := NewInFlightLimiter(2)
	_ = l.Acquire(t.Context())
	_ = l.Acquire(t.Context())
	if !l.Full() || l.InUse() != 2 {
		t.Fatal("limiter should be full")
	}
	l.Release()
	if l.Full() || l.InUse() != 1 {
		t.Fatal("release did not free a slot")
	}
}

func TestOffsetTrackerPositionIsTheFirstUnfinishedOffset(t *testing.T) {
	tr := NewOffsetTracker()
	tp := TopicPartition{Topic: "in", Partition: 0}
	if _, ok := tr.Position(tp); ok {
		t.Fatal("untracked partition has no position")
	}
	for _, off := range []int64{40, 41, 42} {
		tr.Track(tp, off)
	}
	if pos, _ := tr.Position(tp); pos != 40 {
		t.Fatalf("position before any outcome = %d, want 40", pos)
	}
	tr.MarkDone(tp, 41)
	if pos, _ := tr.Position(tp); pos != 40 {
		t.Fatalf("position with 40 unfinished = %d, want 40", pos)
	}
	tr.MarkDone(tp, 40)
	if pos, _ := tr.Position(tp); pos != 42 {
		t.Fatalf("position after 40, 41 = %d, want 42", pos)
	}
}
