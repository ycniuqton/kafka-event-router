package dlq

import (
	"context"
	"sort"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// PartitionScan is the backwards scan position of one partition.
type PartitionScan struct {
	Conn    string
	Range   PartitionRange
	Upper   int64
	Cur     int64
	Matches []record.Record
}

// Exhausted reports whether the scan reached the partition's lower bound.
func (p *PartitionScan) Exhausted() bool { return p.Cur <= p.Range.Lower() }

// Window returns the next window below Cur.
func (p *PartitionScan) Window() kafka.PartitionWindow {
	start := max(p.Range.Lower(), p.Cur-int64(WindowSize))
	return kafka.PartitionWindow{Topic: p.Range.Topic, Partition: p.Range.Partition, Start: start, End: p.Cur}
}

// ScanState holds every partition of a scan.
type ScanState struct {
	parts     map[PartitionKey]*PartitionScan
	scanned   int
	truncated bool
}

// NewScanState positions each partition at min(To, cursor bound); finished cursor partitions stay put.
func NewScanState(conn map[string]RangeSet, cur Cursor) *ScanState {
	s := &ScanState{parts: map[PartitionKey]*PartitionScan{}}
	for c, set := range conn {
		for _, r := range set.Ranges {
			upper := r.To
			if b, ok := cur.Bound(r.Key()); ok {
				upper = min(upper, b)
			}
			s.parts[r.Key()] = &PartitionScan{Conn: c, Range: r, Upper: upper, Cur: upper}
		}
	}
	return s
}

// Windows returns the next window per connection for partitions still needing matches.
func (s *ScanState) Windows(want int) map[string][]kafka.PartitionWindow {
	out := map[string][]kafka.PartitionWindow{}
	for _, p := range s.parts {
		if p.Exhausted() || len(p.Matches) >= want {
			continue
		}
		out[p.Conn] = append(out[p.Conn], p.Window())
	}
	return out
}

// Absorb decodes a round's records, applies filters and moves each read partition below its window.
func (s *ScanState) Absorb(ctx context.Context, windows []kafka.PartitionWindow, raws []engine.RawRecord, req ScanRequest) error {
	byPart := map[PartitionKey][]record.Record{}
	for _, raw := range raws {
		rec, _ := record.Codec{}.Decode(raw)
		s.scanned++
		if !req.Budget.Admit(rec.Size()) {
			s.truncated = true
		}
		if req.Query.Matches(rec) {
			byPart[KeyOf(raw.Topic, raw.Partition)] = append(byPart[KeyOf(raw.Topic, raw.Partition)], rec)
		}
	}
	for _, w := range windows {
		p := s.parts[KeyOf(w.Topic, w.Partition)]
		kept, err := req.Filter.Keep(ctx, NewestFirst{}.Sort(byPart[KeyOf(w.Topic, w.Partition)]))
		if err != nil {
			return err
		}
		p.Matches = append(p.Matches, kept...)
		p.Cur = w.Start
	}
	return nil
}

// MarkTruncated records that the scan stopped on its budget.
func (s *ScanState) MarkTruncated() { s.truncated = true }

// NewestFirst orders records by failure time, newest first (offset breaks ties).
type NewestFirst struct{}

// Sort sorts recs in place and returns them.
func (NewestFirst) Sort(recs []record.Record) []record.Record {
	sort.SliceStable(recs, func(i, j int) bool {
		a, b := recs[i], recs[j]
		if !a.Failure.FailedAt.Equal(b.Failure.FailedAt) {
			return a.Failure.FailedAt.After(b.Failure.FailedAt)
		}
		return a.Ref.Offset > b.Ref.Offset
	})
	return recs
}

// PartitionMerger merges partition matches newest-first and computes the next cursor (design §5.1).
type PartitionMerger struct{}

// Page takes the newest want matches; untaken matches stay above the next cursor bound.
func (PartitionMerger) Page(s *ScanState, want int) ScanPage {
	all := []record.Record{}
	for _, p := range s.parts {
		all = append(all, p.Matches...)
	}
	all = NewestFirst{}.Sort(all)
	if len(all) > want {
		all = all[:want]
	}
	taken := map[PartitionKey][]record.Record{}
	for _, r := range all {
		k := KeyOf(r.Ref.Topic, r.Ref.Partition)
		taken[k] = append(taken[k], r)
	}
	next, done := NewCursor(), true
	for k, p := range s.parts {
		bound := PartitionMerger{}.bound(p, taken[k])
		next.Bounds[k] = bound
		done = done && bound <= p.Range.Lower()
	}
	next.Done = done
	return ScanPage{Records: all, Next: next, Scanned: s.scanned, Truncated: s.truncated}
}

// bound is Cur when every match was taken, else the offset of the oldest taken match (or Upper).
func (PartitionMerger) bound(p *PartitionScan, taken []record.Record) int64 {
	switch {
	case len(taken) == len(p.Matches):
		return p.Cur
	case len(taken) == 0:
		return p.Upper
	}
	oldest := taken[0].Ref.Offset
	for _, r := range taken {
		oldest = min(oldest, r.Ref.Offset)
	}
	return oldest
}
