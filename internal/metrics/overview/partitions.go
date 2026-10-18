package overview

import (
	"sort"

	in "github.com/eventrouter/router/internal/metrics/instruments"
	"github.com/eventrouter/router/internal/metrics/query"
)

// PartitionLimit caps the lag chart's series count (high-cardinality guard).
type PartitionLimit int

const MaxPartitions PartitionLimit = 50

// OtherPartition is the key of the summed remainder.
type OtherPartition string

const PartitionOther OtherPartition = "other"

// PartitionCap keeps the Max partitions with the highest latest lag and sums the rest into "other".
type PartitionCap struct {
	Max int
}

// Apply returns lag points keyed by partition.
func (c PartitionCap) Apply(series []query.Series) map[string][]query.Point {
	sorted := append([]query.Series(nil), series...)
	sort.SliceStable(sorted, func(i, j int) bool { return c.latest(sorted[i]) > c.latest(sorted[j]) })
	out := make(map[string][]query.Point, min(len(sorted), c.Max+1))
	for i, s := range sorted {
		if i < c.Max {
			out[s.Labels[string(in.LabelPartition)]] = s.Points
			continue
		}
		out[string(PartitionOther)] = c.add(out[string(PartitionOther)], s.Points)
	}
	return out
}

func (PartitionCap) latest(s query.Series) float64 {
	if len(s.Points) == 0 {
		return 0
	}
	return s.Points[len(s.Points)-1].Value
}

// add sums b into a point-by-point on matching timestamps.
func (PartitionCap) add(a, b []query.Point) []query.Point {
	if a == nil {
		return append([]query.Point(nil), b...)
	}
	idx := make(map[int64]int, len(a))
	for i, p := range a {
		idx[p.At.Unix()] = i
	}
	for _, p := range b {
		if i, ok := idx[p.At.Unix()]; ok {
			a[i].Value += p.Value
			continue
		}
		a = append(a, p)
	}
	return a
}
