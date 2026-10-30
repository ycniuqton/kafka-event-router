// Package changes relays router_runtime NOTIFY events (0008 trigger) to in-process subscribers: the
// worker SSE stream (GET /internal/v1/changes) and the scheduler loop.
package changes

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/store"
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

// PgListener relays router_runtime notifications from the process listener hub (one dedicated session, not a
// pooled connection) to the broker; a hub reset (reconnect, overflow) publishes a resync hint.
type PgListener struct {
	Notifications store.NotificationSource
	Broker        *Broker
}

// Run relays until ctx ends.
func (l PgListener) Run(ctx context.Context) {
	sub := l.Notifications.Subscribe(store.Channel(domain.ChannelRouterRuntime))
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case n, ok := <-sub.C():
			if !ok {
				return
			}
			l.relay(n)
		}
	}
}

// relay turns one hub notice into a hint (unparsable payloads are ignored).
func (l PgListener) relay(n store.Notice) {
	if n.Kind == store.NoticeReset {
		l.Broker.Publish(domain.ChangeHint{Kind: domain.ChangeResync})
		return
	}
	var p notifyPayload
	if json.Unmarshal([]byte(n.Payload), &p) == nil {
		l.Broker.Publish(domain.ChangeHint{RouteID: p.RouteID, Kind: p.Kind})
	}
}
