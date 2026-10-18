// Package changes relays router_runtime NOTIFY events (0008 trigger) to in-process subscribers: the
// worker SSE stream (GET /internal/v1/changes) and the scheduler loop.
package changes

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/retry"
)

// BufferSize is the per-subscriber hint buffer; slow subscribers drop hints (they also resync on timers).
type BufferSize int

const SubscriberBuffer BufferSize = 64

// Broker fans change hints out to subscribers.
type Broker struct {
	mu   sync.Mutex
	subs map[chan domain.ChangeHint]struct{}
}

// NewBroker returns an empty broker.
func NewBroker() *Broker {
	return &Broker{subs: map[chan domain.ChangeHint]struct{}{}}
}

// Subscribe returns a hint channel and its cancel function.
func (b *Broker) Subscribe() (<-chan domain.ChangeHint, func()) {
	ch := make(chan domain.ChangeHint, SubscriberBuffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
}

// Publish delivers a hint to every subscriber without blocking.
func (b *Broker) Publish(h domain.ChangeHint) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- h:
		default:
		}
	}
}

// notifyPayload is the JSON body the router_runtime_notify trigger sends.
type notifyPayload struct {
	RouteID string            `json:"route_id"`
	Kind    domain.ChangeKind `json:"kind"`
}

// PgListener LISTENs on router_runtime on a dedicated connection and publishes hints; it reconnects with backoff.
type PgListener struct {
	Pool   *pgxpool.Pool
	Broker *Broker
	Retry  retry.Policy
	Log    *slog.Logger
}

// Run listens until ctx ends.
func (l PgListener) Run(ctx context.Context) {
	for attempt := 0; ctx.Err() == nil; attempt++ {
		if err := l.listen(ctx); err != nil && ctx.Err() == nil {
			l.Log.Warn("runtime: change listener reconnecting", string(logging.AttrError), err)
		}
		l.Broker.Publish(domain.ChangeHint{Kind: domain.ChangeResync})
		if l.Retry.Wait(ctx, attempt) != nil {
			return
		}
	}
}

func (l PgListener) listen(ctx context.Context) error {
	conn, err := l.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN "+string(domain.ChannelRouterRuntime)); err != nil {
		return err
	}
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var p notifyPayload
		if json.Unmarshal([]byte(n.Payload), &p) == nil {
			l.Broker.Publish(domain.ChangeHint{RouteID: p.RouteID, Kind: p.Kind})
		}
	}
}
