package scheduler

import (
	"context"
	"time"
)

// LeaseStore deletes expired leases.
type LeaseStore interface {
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}

// LeaseReaper removes leases of workers that stopped heartbeating (lease older than LeaseTTL); the
// planner places replacements. A partitioned old worker keeps running until it gets an explicit answer.
type LeaseReaper struct {
	Leases LeaseStore
	Now    func() time.Time
}

// Reap deletes expired leases and returns how many.
func (r LeaseReaper) Reap(ctx context.Context) (int, error) {
	return r.Leases.DeleteExpired(ctx, r.Now())
}
