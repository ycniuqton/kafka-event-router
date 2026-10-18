package kafka

import (
	"testing"
	"time"
)

func marks(parts ...[2]int64) []PartitionMarks {
	out := []PartitionMarks{}
	for i, p := range parts {
		out = append(out, PartitionMarks{Topic: "t", Partition: int32(i), Low: p[0], High: p[1], FromOffset: int64(OffsetUnresolved)})
	}
	return out
}

func total(ws []PartitionWindow) int64 {
	n := int64(0)
	for _, w := range ws {
		n += w.Len()
	}
	return n
}

func TestLatestOffsetPlanner(t *testing.T) {
	cases := []struct {
		name      string
		count     int
		marks     []PartitionMarks
		windows   int
		atLeast   int64
		perWindow map[int32]int64
	}{
		{name: "empty topic", count: 50, marks: marks([2]int64{0, 0}, [2]int64{5, 5}), windows: 0},
		{name: "even split", count: 50, marks: marks([2]int64{0, 100}, [2]int64{0, 100}), windows: 2, atLeast: 50, perWindow: map[int32]int64{0: 25, 1: 25}},
		{name: "skewed partitions refill", count: 50, marks: marks([2]int64{0, 3}, [2]int64{0, 1000}), windows: 2, atLeast: 50, perWindow: map[int32]int64{0: 3, 1: 47}},
		{name: "n smaller than partitions", count: 2, marks: marks([2]int64{0, 10}, [2]int64{0, 10}, [2]int64{0, 10}), windows: 3, atLeast: 2, perWindow: map[int32]int64{0: 1, 1: 1, 2: 1}},
		{name: "fewer records than asked", count: 50, marks: marks([2]int64{10, 20}), windows: 1, atLeast: 10, perWindow: map[int32]int64{0: 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := LatestOffsetPlanner{}.Plan(SamplingPlan{Count: tc.count}, tc.marks)
			if len(ws) != tc.windows {
				t.Fatalf("windows = %d, want %d (%+v)", len(ws), tc.windows, ws)
			}
			if total(ws) < tc.atLeast {
				t.Fatalf("total = %d, want >= %d", total(ws), tc.atLeast)
			}
			for _, w := range ws {
				if want, ok := tc.perWindow[w.Partition]; ok && w.Len() != want {
					t.Errorf("partition %d len = %d, want %d", w.Partition, w.Len(), want)
				}
				if w.End != tc.marks[w.Partition].High {
					t.Errorf("partition %d end = %d, want HWM %d", w.Partition, w.End, tc.marks[w.Partition].High)
				}
			}
		})
	}
}

func TestTimeRangeOffsetPlanner(t *testing.T) {
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ms := []PartitionMarks{
		{Topic: "t", Partition: 0, Low: 0, High: 100, FromOffset: 40},
		{Topic: "t", Partition: 1, Low: 50, High: 60, FromOffset: 10},
		{Topic: "t", Partition: 2, Low: 0, High: 10, FromOffset: int64(OffsetUnresolved)},
		{Topic: "t", Partition: 3, Low: 0, High: 10, FromOffset: 10},
	}
	ws := TimeRangeOffsetPlanner{}.Plan(SamplingPlan{To: to}, ms)
	if len(ws) != 2 {
		t.Fatalf("windows = %+v", ws)
	}
	if ws[0].Start != 40 || ws[0].End != 100 || !ws[0].Until.Equal(to) {
		t.Errorf("p0 window = %+v", ws[0])
	}
	if ws[1].Start != 50 || ws[1].End != 60 {
		t.Errorf("p1 window clamps to low watermark: %+v", ws[1])
	}
}

func TestSamplingBudget(t *testing.T) {
	now := time.Unix(100, 0)
	cases := []struct {
		name   string
		budget *SamplingBudget
		sizes  []int64
		admit  int
		reason StopReason
	}{
		{name: "complete", budget: NewSamplingBudget(10, 100, time.Time{}), sizes: []int64{10, 10}, admit: 2, reason: StopComplete},
		{name: "max events", budget: NewSamplingBudget(2, 100, time.Time{}), sizes: []int64{1, 1, 1}, admit: 2, reason: StopMaxEvents},
		{name: "max bytes", budget: NewSamplingBudget(10, 15, time.Time{}), sizes: []int64{10, 10}, admit: 1, reason: StopMaxBytes},
		{name: "deadline", budget: NewSamplingBudget(10, 100, now), sizes: []int64{1}, admit: 0, reason: StopDeadline},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			admitted := 0
			for _, s := range tc.sizes {
				if tc.budget.Admit(s, now) {
					admitted++
				}
			}
			if admitted != tc.admit || tc.budget.Reason() != tc.reason {
				t.Fatalf("admitted %d reason %s, want %d %s", admitted, tc.budget.Reason(), tc.admit, tc.reason)
			}
			if tc.reason.Truncated() != (tc.reason != StopComplete) {
				t.Fatal("Truncated mismatch")
			}
		})
	}
}
