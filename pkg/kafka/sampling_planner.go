package kafka

import "sort"

// OffsetPlanner turns a sampling plan and partition marks into read windows (pure).
type OffsetPlanner interface {
	Plan(plan SamplingPlan, marks []PartitionMarks) []PartitionWindow
}

// LatestOffsetPlanner reads the newest records: every partition gets at least ceil(N/p) and
// the quota short partitions cannot fill is handed to the larger ones.
type LatestOffsetPlanner struct{}

// Plan implements OffsetPlanner.
func (LatestOffsetPlanner) Plan(plan SamplingPlan, marks []PartitionMarks) []PartitionWindow {
	nonEmpty := LatestOffsetPlanner{}.nonEmpty(marks)
	if plan.Count <= 0 || len(nonEmpty) == 0 {
		return []PartitionWindow{}
	}
	sort.SliceStable(nonEmpty, func(i, j int) bool { return nonEmpty[i].Available() < nonEmpty[j].Available() })
	floor := SamplingQuota{}.Ceil(int64(plan.Count), int64(len(nonEmpty)))
	remaining := int64(plan.Count)
	windows := make([]PartitionWindow, 0, len(nonEmpty))
	for i, m := range nonEmpty {
		share := max(floor, SamplingQuota{}.Ceil(remaining, int64(len(nonEmpty)-i)))
		take := min(share, m.Available())
		remaining = max(0, remaining-take)
		windows = append(windows, PartitionWindow{Topic: m.Topic, Partition: m.Partition, Start: m.High - take, End: m.High})
	}
	return SamplingOrder{}.Sort(windows)
}

func (LatestOffsetPlanner) nonEmpty(marks []PartitionMarks) []PartitionMarks {
	out := make([]PartitionMarks, 0, len(marks))
	for _, m := range marks {
		if m.Available() > 0 {
			out = append(out, m)
		}
	}
	return out
}

// TimeRangeOffsetPlanner reads from the first offset at or after From up to the HWM snapshot,
// stopping at records newer than To; partitions without records after From are skipped.
type TimeRangeOffsetPlanner struct{}

// Plan implements OffsetPlanner.
func (TimeRangeOffsetPlanner) Plan(plan SamplingPlan, marks []PartitionMarks) []PartitionWindow {
	windows := make([]PartitionWindow, 0, len(marks))
	for _, m := range marks {
		if m.FromOffset == int64(OffsetUnresolved) || m.FromOffset >= m.High {
			continue
		}
		w := PartitionWindow{Topic: m.Topic, Partition: m.Partition, Start: max(m.Low, m.FromOffset), End: m.High, Until: plan.To}
		if w.Len() > 0 {
			windows = append(windows, w)
		}
	}
	return SamplingOrder{}.Sort(windows)
}

// SamplingQuota splits event quotas across partitions.
type SamplingQuota struct{}

// Ceil returns ceil(n/d) for positive d, else 0.
func (SamplingQuota) Ceil(n, d int64) int64 {
	if d <= 0 || n <= 0 {
		return 0
	}
	return (n + d - 1) / d
}

// SamplingOrder gives windows a stable topic/partition order.
type SamplingOrder struct{}

// Sort orders windows by topic then partition.
func (SamplingOrder) Sort(ws []PartitionWindow) []PartitionWindow {
	sort.SliceStable(ws, func(i, j int) bool {
		if ws[i].Topic != ws[j].Topic {
			return ws[i].Topic < ws[j].Topic
		}
		return ws[i].Partition < ws[j].Partition
	})
	return ws
}
