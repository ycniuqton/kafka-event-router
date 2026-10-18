package dlq

import (
	"context"
	"sort"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/kafka"
)

// KafkaReader implements Reader over a PartitionSource: backwards windows, budgets, k-way merge.
type KafkaReader struct {
	Source PartitionSource
}

// Scan implements Reader.
func (r KafkaReader) Scan(ctx context.Context, targets []Target, req ScanRequest) (ScanPage, error) {
	ctx, cancel := context.WithDeadline(ctx, req.Budget.Deadline())
	defer cancel()
	sets, missing, err := r.rangeSets(ctx, targets, req.Query.Since, req.Query.Until)
	if err != nil {
		return ScanPage{}, err
	}
	state := NewScanState(sets, req.Cursor)
	for round := state.Windows(req.Want); len(round) > 0; round = state.Windows(req.Want) {
		if req.Budget.Exhausted() {
			state.MarkTruncated()
			break
		}
		if err := r.readRound(ctx, round, state, req); err != nil {
			return ScanPage{}, err
		}
	}
	page := PartitionMerger{}.Page(state, req.Want)
	page.Missing = missing
	return page, nil
}

// readRound reads one window per open partition, connection by connection.
func (r KafkaReader) readRound(ctx context.Context, round map[string][]kafka.PartitionWindow, state *ScanState, req ScanRequest) error {
	for conn, windows := range round {
		raws, err := r.Source.Read(ctx, conn, windows)
		if err != nil && ctx.Err() == nil {
			return err
		}
		if ctx.Err() != nil {
			state.MarkTruncated()
		}
		if err := state.Absorb(ctx, windows, raws, req); err != nil {
			return err
		}
	}
	return nil
}

// rangeSets lists the partitions of every target.
func (r KafkaReader) rangeSets(ctx context.Context, targets []Target, since, until time.Time) (map[string]RangeSet, []string, error) {
	sets, missing := map[string]RangeSet{}, []string{}
	for _, t := range targets {
		set, err := r.Source.Ranges(ctx, t, since, until)
		if err != nil {
			return nil, nil, err
		}
		prev := sets[t.ConnectionID]
		sets[t.ConnectionID] = RangeSet{Ranges: append(prev.Ranges, set.Ranges...)}
		missing = append(missing, set.Missing...)
	}
	return sets, missing, nil
}

// Ranges implements Reader: every partition of every target.
func (r KafkaReader) Ranges(ctx context.Context, targets []Target) (RangeSet, error) {
	sets, missing, err := r.rangeSets(ctx, targets, time.Time{}, time.Time{})
	if err != nil {
		return RangeSet{}, err
	}
	out := RangeSet{Missing: missing}
	for _, s := range sets {
		out.Ranges = append(out.Ranges, s.Ranges...)
	}
	return out, nil
}

// Fetch implements Reader: single-offset reads; refs below the low watermark are expired.
func (r KafkaReader) Fetch(ctx context.Context, targets []Target, refs []Ref) (FetchResult, error) {
	res := FetchResult{Records: map[Ref]record.Record{}}
	for _, t := range targets {
		wanted := RefsOf{}.Target(t, refs)
		if len(wanted) == 0 {
			continue
		}
		if err := r.fetchTarget(ctx, t, wanted, &res); err != nil {
			return FetchResult{}, err
		}
	}
	for _, ref := range refs {
		if _, ok := res.Records[ref]; !ok && !(RefsOf{}).Contains(res.Expired, ref) {
			res.Missing = append(res.Missing, ref)
		}
	}
	return res, nil
}

func (r KafkaReader) fetchTarget(ctx context.Context, t Target, refs []Ref, res *FetchResult) error {
	set, err := r.Source.Ranges(ctx, t, time.Time{}, time.Time{})
	if err != nil {
		return err
	}
	lows := map[PartitionKey]PartitionRange{}
	for _, pr := range set.Ranges {
		lows[pr.Key()] = pr
	}
	readable := []Ref{}
	for _, ref := range refs {
		pr, ok := lows[KeyOf(ref.Topic, ref.Partition)]
		switch {
		case ok && ref.Offset < pr.Low:
			res.Expired = append(res.Expired, ref)
		case ok && ref.Offset < pr.High:
			readable = append(readable, ref)
		}
	}
	return r.readRefs(ctx, t.ConnectionID, readable, res)
}

// readRefs reads the spans covering refs in batches and keeps only the requested offsets.
func (r KafkaReader) readRefs(ctx context.Context, conn string, refs []Ref, res *FetchResult) error {
	wanted := map[Ref]bool{}
	for _, ref := range refs {
		wanted[ref] = true
	}
	for _, batch := range (WindowBatches{}).Split((RefSpans{}).Windows(refs)) {
		raws, err := r.Source.Read(ctx, conn, batch)
		if err != nil {
			return err
		}
		for _, raw := range raws {
			rec, _ := record.Codec{}.Decode(raw)
			if wanted[rec.Ref] {
				res.Records[rec.Ref] = rec
			}
		}
	}
	return nil
}

// RefSpans turns refs into windows: refs of one partition closer than WindowSize share a window.
type RefSpans struct{}

// Windows returns the covering windows.
func (RefSpans) Windows(refs []Ref) []kafka.PartitionWindow {
	sorted := append([]Ref(nil), refs...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Topic != sorted[j].Topic || sorted[i].Partition != sorted[j].Partition {
			return KeyOf(sorted[i].Topic, sorted[i].Partition) < KeyOf(sorted[j].Topic, sorted[j].Partition)
		}
		return sorted[i].Offset < sorted[j].Offset
	})
	out := []kafka.PartitionWindow{}
	for _, ref := range sorted {
		last := len(out) - 1
		if last >= 0 && out[last].Topic == ref.Topic && out[last].Partition == ref.Partition && ref.Offset-out[last].End < int64(WindowSize) {
			out[last].End = ref.Offset + 1
			continue
		}
		out = append(out, kafka.PartitionWindow{Topic: ref.Topic, Partition: ref.Partition, Start: ref.Offset, End: ref.Offset + 1})
	}
	return out
}

// WindowBatches splits windows so each batch holds at most one window per partition.
type WindowBatches struct{}

// Split groups windows into batches with distinct partitions.
func (WindowBatches) Split(windows []kafka.PartitionWindow) [][]kafka.PartitionWindow {
	batches := [][]kafka.PartitionWindow{}
	used := []map[PartitionKey]bool{}
	for _, w := range windows {
		k := KeyOf(w.Topic, w.Partition)
		i := 0
		for i < len(batches) && used[i][k] {
			i++
		}
		if i == len(batches) {
			batches, used = append(batches, nil), append(used, map[PartitionKey]bool{})
		}
		batches[i], used[i][k] = append(batches[i], w), true
	}
	return batches
}

// RefsOf filters ref lists.
type RefsOf struct{}

// Target returns the refs whose topic belongs to t.
func (RefsOf) Target(t Target, refs []Ref) []Ref {
	out := []Ref{}
	for _, r := range refs {
		if t.Has(r.Topic) {
			out = append(out, r)
		}
	}
	return out
}

// Contains reports whether ref is in refs.
func (RefsOf) Contains(refs []Ref, ref Ref) bool {
	for _, r := range refs {
		if r == ref {
			return true
		}
	}
	return false
}
