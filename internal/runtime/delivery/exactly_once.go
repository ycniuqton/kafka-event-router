package delivery

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/internal/runtime/producers"
	"github.com/eventrouter/router/pkg/config"
)

// ErrCrossConnection is returned when an exactly_once route touches a connection other than its source:
// outputs, dead letters and offsets must share one cluster to share one transaction.
var ErrCrossConnection = errors.New("exactly_once requires source, destinations and DLQ on one connection")

// ErrNoSession is returned when an exactly_once runner was opened without a transact session.
var ErrNoSession = errors.New("exactly_once requires a transactional source session")

// ExactlyOnceFactory builds ExactlyOnceStrategy after the start-time same-connection guard.
type ExactlyOnceFactory struct{}

func (ExactlyOnceFactory) Mode() config.DeliveryMode { return config.DeliveryModeExactlyOnce }

// New re-checks that every connection of the route is the source connection and wraps the session.
func (ExactlyOnceFactory) New(_ context.Context, d Deps) (Strategy, error) {
	if err := (SameConnectionGuard{}).Check(d); err != nil {
		return nil, err
	}
	if d.Session == nil {
		return nil, ErrNoSession
	}
	abort := d.Abort
	if abort == nil {
		abort = func() {}
	}
	return &ExactlyOnceStrategy{session: d.Session, source: d.Source, abort: abort}, nil
}

// SameConnectionGuard is the runner-start re-check of pipeline-engine's DeliveryModeValidator.
type SameConnectionGuard struct{}

// Check fails when any resolved connection (destination or DLQ) differs from the source.
func (SameConnectionGuard) Check(d Deps) error {
	for id := range d.Connections {
		if id != d.Source {
			return fmt.Errorf("%w: connection %q differs from source %q", ErrCrossConnection, id, d.Source)
		}
	}
	return nil
}

// ExactlyOnceStrategy produces outputs and dead letters inside the source session's transaction; the
// session commits the polled offsets in the same transaction, and an abort rewinds to the last commit.
type ExactlyOnceStrategy struct {
	session *kgo.GroupTransactSession
	source  string
	abort   func()
	failed  atomic.Bool
}

func (*ExactlyOnceStrategy) Mode() config.DeliveryMode { return config.DeliveryModeExactlyOnce }

// Batching implements Strategy: one transaction per polled batch.
func (*ExactlyOnceStrategy) Batching() BatchMode { return BatchTransactional }

// Produce adds out to the open transaction; any failure dooms the batch, which is then aborted and re-read.
func (s *ExactlyOnceStrategy) Produce(ctx context.Context, out Outgoing, onAck AckFunc) {
	if out.ConnectionID != s.source {
		onAck(ErrCrossConnection)
		return
	}
	rec := RecordMapper{}.ToKafka(out.Record)
	rec.Context = producers.PartitionModes{}.Tag(context.Background(), out.Partitioning)
	s.session.Produce(ctx, rec, func(_ *kgo.Record, err error) {
		if err != nil {
			s.fail()
		}
		onAck(err)
	})
}

// Commit implements Strategy: offsets travel inside the transaction, so explicit commits are no-ops.
func (*ExactlyOnceStrategy) Commit(context.Context, []Mark) error { return nil }

// BeginBatch opens the transaction of one polled batch.
func (s *ExactlyOnceStrategy) BeginBatch(context.Context) error {
	s.failed.Store(false)
	return s.session.Begin()
}

// EndBatch commits outputs and offsets together, or aborts (rewinding the consumer) when commit is
// false or a produce of the batch failed.
func (s *ExactlyOnceStrategy) EndBatch(ctx context.Context, commit bool) (bool, error) {
	try := kgo.TransactionEndTry(commit && !s.failed.Load())
	return s.session.End(ctx, try)
}

// Close implements Strategy; the session is closed with the source consumer that owns it.
func (*ExactlyOnceStrategy) Close(context.Context) error { return nil }

func (s *ExactlyOnceStrategy) fail() {
	if s.failed.CompareAndSwap(false, true) {
		s.abort()
	}
}
