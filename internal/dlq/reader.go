package dlq

import (
	"context"
	"sync"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
)

// RecordFilter drops records during a scan (resolved-state lookup); it sees one batch at a time.
type RecordFilter interface {
	Keep(ctx context.Context, recs []record.Record) ([]record.Record, error)
}

// KeepAll is the filter that keeps every record.
type KeepAll struct{}

// Keep implements RecordFilter.
func (KeepAll) Keep(_ context.Context, recs []record.Record) ([]record.Record, error) {
	return recs, nil
}

// ScanBudget bounds one scan by bytes and a deadline; safe for concurrent use.
type ScanBudget struct {
	mu       sync.Mutex
	maxBytes int64
	deadline time.Time
	bytes    int64
	hit      bool
	now      func() time.Time
}

// NewScanBudget starts a budget with the feature limits.
func NewScanBudget(now func() time.Time) *ScanBudget {
	return &ScanBudget{maxBytes: int64(MaxScanBytes), deadline: now().Add(MaxScanTime.Duration()), now: now}
}

// Admit counts size bytes; it returns false once a limit is hit.
func (b *ScanBudget) Admit(size int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bytes += size
	if b.bytes > b.maxBytes || !b.now().Before(b.deadline) {
		b.hit = true
	}
	return !b.hit
}

// Exhausted reports whether a limit was hit (checking the deadline now).
func (b *ScanBudget) Exhausted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.now().Before(b.deadline) {
		b.hit = true
	}
	return b.hit
}

// Deadline returns when the scan must stop.
func (b *ScanBudget) Deadline() time.Time { return b.deadline }

// ScanRequest is one page request over a location's targets.
type ScanRequest struct {
	Query  Query
	Cursor Cursor
	Want   int
	Filter RecordFilter
	Budget *ScanBudget
}

// ScanPage is the newest-first result of a scan.
type ScanPage struct {
	Records   []record.Record
	Next      Cursor
	Scanned   int
	Truncated bool
	Missing   []string
}

// FetchResult is the outcome of reading records by ref.
type FetchResult struct {
	Records map[Ref]record.Record
	Expired []Ref
	Missing []Ref
}

// Reader reads DLQ topics without a consumer group.
type Reader interface {
	Scan(ctx context.Context, targets []Target, req ScanRequest) (ScanPage, error)
	Fetch(ctx context.Context, targets []Target, refs []Ref) (FetchResult, error)
	Ranges(ctx context.Context, targets []Target) (RangeSet, error)
}
