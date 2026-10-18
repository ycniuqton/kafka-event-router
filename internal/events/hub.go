package events

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/logging"
)

// ClientBuffer is the per-socket queue; when full, events are dropped and the client is told to resync.
type ClientBuffer int

const DefaultClientBuffer ClientBuffer = 64

// HubRetry is the wait before re-subscribing to the bus after a failure.
type HubRetry time.Duration

const BusRetryDelay HubRetry = HubRetry(5 * time.Second)

// Client is one connected socket.
type Client struct {
	principal auth.Principal
	queue     chan PushEvent
	dropped   atomic.Bool
}

// Queue returns the events to send.
func (c *Client) Queue() <-chan PushEvent {
	return c.queue
}

// TakeDropped reports (and clears) whether events were dropped since the last call.
func (c *Client) TakeDropped() bool {
	return c.dropped.Swap(false)
}

// Hub is the per-instance fan-out of push events to sockets, filtered by the visibility policy.
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]struct{}
	visibility VisibilityPolicy
	log        *slog.Logger
}

// NewHub builds an empty hub.
func NewHub(v VisibilityPolicy, log *slog.Logger) *Hub {
	return &Hub{clients: map[*Client]struct{}{}, visibility: v, log: log}
}

// Register adds a socket of principal p.
func (h *Hub) Register(p auth.Principal) *Client {
	c := &Client{principal: p, queue: make(chan PushEvent, DefaultClientBuffer)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

// Unregister removes a socket.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// Clients returns the number of connected sockets.
func (h *Hub) Clients() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Publish delivers e to this instance's visible sockets (local only; implements Publisher).
func (h *Hub) Publish(_ context.Context, e PushEvent) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if !h.visibility.Allows(c.principal, e.TeamID) {
			continue
		}
		select {
		case c.queue <- e:
		default:
			c.dropped.Store(true)
		}
	}
	return nil
}

// Run feeds bus events into the hub until ctx ends, re-subscribing after failures.
func (h *Hub) Run(ctx context.Context, bus Bus) {
	for ctx.Err() == nil {
		ch, err := bus.Subscribe(ctx)
		if err != nil {
			h.log.Warn("events: bus subscribe failed", string(logging.AttrError), err)
		} else {
			for e := range ch {
				_ = h.Publish(ctx, e)
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(BusRetryDelay)):
		}
	}
}

// FallbackPublisher publishes on the bus and, when the bus fails (Redis down), delivers locally.
type FallbackPublisher struct {
	Bus   Bus
	Local Publisher
	Log   *slog.Logger
}

// Publish implements Publisher.
func (p FallbackPublisher) Publish(ctx context.Context, e PushEvent) error {
	if err := p.Bus.Publish(ctx, e); err != nil {
		p.Log.Warn("events: bus publish failed, delivering locally", string(logging.AttrError), err)
		return p.Local.Publish(ctx, e)
	}
	return nil
}
