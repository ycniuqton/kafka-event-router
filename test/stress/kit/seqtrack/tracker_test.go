package seqtrack

import "testing"

func TestInOrderStreamIsComplete(t *testing.T) {
	tr := New()
	for i := int64(0); i < 100; i++ {
		tr.Observe("a", i)
		tr.Observe("b", i)
	}
	s := tr.Summary(200)
	if s.Unique != 200 || s.Duplicates != 0 || s.Gaps != 0 || s.Lost != 0 || s.Streams != 2 {
		t.Errorf("summary = %+v", s)
	}
}

func TestDuplicatesAndReplays(t *testing.T) {
	tr := New()
	for _, seq := range []int64{0, 1, 2, 3, 2, 3, 4, 4} {
		tr.Observe("k", seq)
	}
	s := tr.Summary(5)
	if s.Duplicates != 3 || s.Unique != 5 || s.Received != 8 || s.Gaps != 0 {
		t.Errorf("summary = %+v", s)
	}
}

func TestGapsAndLateArrival(t *testing.T) {
	tr := New()
	for _, seq := range []int64{0, 3, 5} {
		tr.Observe("k", seq)
	}
	if s := tr.Summary(0); s.Gaps != 3 || s.Unique != 3 {
		t.Errorf("before fill = %+v", s)
	}
	tr.Observe("k", 1)
	tr.Observe("k", 2)
	tr.Observe("k", 3)
	s := tr.Summary(7)
	if s.Gaps != 1 || s.Unique != 5 || s.OutOfOrder != 2 || s.Duplicates != 1 || s.Lost != 2 {
		t.Errorf("after fill = %+v", s)
	}
}

func TestDuplicateAboveWatermarkDetected(t *testing.T) {
	tr := New()
	tr.Observe("k", 4)
	tr.Observe("k", 4)
	if s := tr.Summary(0); s.Duplicates != 1 || s.Gaps != 4 {
		t.Errorf("summary = %+v", s)
	}
}
