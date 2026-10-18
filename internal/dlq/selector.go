package dlq

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/platform/problem"
)

// SelectionCap bounds a selection: ids above Limit fail with Err; matches above Limit are
// truncated (Remaining reported) when Truncate, else fail with Err.
type SelectionCap struct {
	Limit    int
	Err      *problem.Error
	Truncate bool
}

// ActionCap is the cap of synchronous retry/discard (FR-13).
var ActionCap = SelectionCap{Limit: int(MaxSyncAction), Err: ErrSelectionTooBig, Truncate: true}

// ExportCap is the cap of one export (FR-17).
var ExportCap = SelectionCap{Limit: int(MaxExport), Err: ErrExportTooBig}

// Resolved is a selection turned into records.
type Resolved struct {
	Records   []record.Record
	Skipped   []ItemResult
	Remaining int
	Truncated bool
}

// Selector resolves a Selection into records (design §5.5).
type Selector struct {
	Reader Reader
	Gate   StateGate
	Now    func() time.Time
}

// Resolve returns the selected records within the cap.
func (s Selector) Resolve(ctx context.Context, loc Location, sel Selection, cap SelectionCap) (Resolved, error) {
	if err := sel.Validate(); err != nil {
		return Resolved{}, err
	}
	if sel.Match != nil {
		return s.match(ctx, loc, sel, cap)
	}
	return s.ids(ctx, loc, sel.IDs, cap)
}

func (s Selector) ids(ctx context.Context, loc Location, ids []string, cap SelectionCap) (Resolved, error) {
	if len(ids) > cap.Limit {
		return Resolved{}, cap.Err.At(string(PathIDs))
	}
	refs, err := RefCodec{}.DecodeAll(ids)
	if err != nil {
		return Resolved{}, err
	}
	res := Resolved{}
	known := []Ref{}
	for _, ref := range refs {
		if _, ok := loc.TargetOf(ref.Topic); !ok {
			res.Skipped = append(res.Skipped, ItemResult{Ref: ref, Outcome: OutcomeSkipped, Reason: SkipNotFound})
			continue
		}
		known = append(known, ref)
	}
	fetched, err := s.Reader.Fetch(ctx, loc.Targets, known)
	if err != nil {
		return Resolved{}, err
	}
	for _, ref := range known {
		if rec, ok := fetched.Records[ref]; ok {
			res.Records = append(res.Records, rec)
		}
	}
	res.Skipped = append(res.Skipped, SkippedOf{}.Refs(fetched.Expired, SkipExpired)...)
	res.Skipped = append(res.Skipped, SkippedOf{}.Refs(fetched.Missing, SkipNotFound)...)
	return res, nil
}

func (s Selector) match(ctx context.Context, loc Location, sel Selection, cap SelectionCap) (Resolved, error) {
	q := sel.Match.WithoutPaging()
	page, err := s.Reader.Scan(ctx, loc.Targets, ScanRequest{
		Query: q, Cursor: NewCursor(), Want: int(MaxExport) + 1, Budget: NewScanBudget(s.Now),
		Filter: StateFilter{Gate: s.Gate, Location: loc, Include: q.IncludeResolved},
	})
	if err != nil {
		return Resolved{}, err
	}
	if sel.ExpectedCount != nil && *sel.ExpectedCount != len(page.Records) {
		return Resolved{}, ErrCountMismatch
	}
	if len(page.Records) <= cap.Limit {
		return Resolved{Records: page.Records, Truncated: page.Truncated}, nil
	}
	if !cap.Truncate {
		return Resolved{}, cap.Err.At(string(PathQuery))
	}
	return Resolved{Records: page.Records[:cap.Limit], Remaining: len(page.Records) - cap.Limit, Truncated: page.Truncated}, nil
}

// SkippedOf builds skipped results.
type SkippedOf struct{}

// Refs returns a skipped result per ref.
func (SkippedOf) Refs(refs []Ref, reason SkipReason) []ItemResult {
	out := make([]ItemResult, 0, len(refs))
	for _, r := range refs {
		out = append(out, ItemResult{Ref: r, Outcome: OutcomeSkipped, Reason: reason})
	}
	return out
}
