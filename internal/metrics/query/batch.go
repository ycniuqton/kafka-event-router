package query

import (
	"context"
	"sync"
)

// Batch runs several queries against one Querier in parallel and reports the first failure.
type Batch struct {
	q     Querier
	tasks []func(ctx context.Context) error
}

// NewBatch starts an empty batch.
func NewBatch(q Querier) *Batch {
	return &Batch{q: q}
}

// Instant schedules an instant query whose result is stored in dst.
func (b *Batch) Instant(e Expr, dst *Vector) *Batch {
	b.tasks = append(b.tasks, func(ctx context.Context) error {
		v, err := b.q.Instant(ctx, e)
		*dst = v
		return err
	})
	return b
}

// Range schedules a range query whose result is stored in dst.
func (b *Batch) Range(e Expr, w Window, dst *[]Series) *Batch {
	b.tasks = append(b.tasks, func(ctx context.Context) error {
		s, err := b.q.Range(ctx, e, w)
		*dst = s
		return err
	})
	return b
}

// Run executes every scheduled query and waits; the first error wins.
func (b *Batch) Run(ctx context.Context) error {
	errs := make([]error, len(b.tasks))
	var wg sync.WaitGroup
	for i, task := range b.tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = task(ctx)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
