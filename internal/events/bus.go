package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/platform/logging"
)

// BusChannel is the Redis pub/sub channel of push events.
type BusChannel string

const DefaultBusChannel BusChannel = "router:events"

// BusBuffer is the per-subscription buffer; a slow subscriber drops events.
type BusBuffer int

const SubscriptionBuffer BusBuffer = 256

// MemoryBus delivers events inside one process (tests, single instance, Redis outage fallback).
type MemoryBus struct {
	mu   sync.Mutex
	subs map[chan PushEvent]struct{}
}

// NewMemoryBus returns an empty bus.
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{subs: map[chan PushEvent]struct{}{}}
}

func (b *MemoryBus) Kind() BusKind { return BusMemory }

// Publish delivers e to every subscriber without blocking.
func (b *MemoryBus) Publish(_ context.Context, e PushEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
	return nil
}

// Subscribe returns a channel closed when ctx ends.
func (b *MemoryBus) Subscribe(ctx context.Context) (<-chan PushEvent, error) {
	ch := make(chan PushEvent, SubscriptionBuffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subs, ch)
		close(ch)
		b.mu.Unlock()
	}()
	return ch, nil
}

// RedisBus publishes on a Redis pub/sub channel so every API instance receives every event.
type RedisBus struct {
	Client  redis.UniversalClient
	Channel BusChannel
	Log     *slog.Logger
}

func (b *RedisBus) Kind() BusKind { return BusRedis }

// Publish implements Publisher.
func (b *RedisBus) Publish(ctx context.Context, e PushEvent) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return b.Client.Publish(ctx, string(b.Channel), body).Err()
}

// Subscribe relays pub/sub messages until ctx ends (go-redis reconnects the subscription itself).
func (b *RedisBus) Subscribe(ctx context.Context) (<-chan PushEvent, error) {
	sub := b.Client.Subscribe(ctx, string(b.Channel))
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return nil, err
	}
	out := make(chan PushEvent, SubscriptionBuffer)
	go b.relay(ctx, sub, out)
	return out, nil
}

func (b *RedisBus) relay(ctx context.Context, sub *redis.PubSub, out chan<- PushEvent) {
	defer close(out)
	defer sub.Close()
	msgs := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-msgs:
			if !ok {
				return
			}
			b.forward(m, out)
		}
	}
}

func (b *RedisBus) forward(m *redis.Message, out chan<- PushEvent) {
	var e PushEvent
	if err := json.Unmarshal([]byte(m.Payload), &e); err != nil {
		b.Log.Warn("events: malformed bus message", string(logging.AttrError), err)
		return
	}
	select {
	case out <- e:
	default:
	}
}

// BusFactory builds one Bus kind.
type BusFactory func() Bus

// BusManager selects the configured Bus implementation.
type BusManager struct {
	factories map[BusKind]BusFactory
}

// NewBusManager registers the built-in buses; Redis is only offered when a client exists.
func NewBusManager(client redis.UniversalClient, log *slog.Logger) *BusManager {
	m := &BusManager{factories: map[BusKind]BusFactory{
		BusMemory: func() Bus { return NewMemoryBus() },
	}}
	if client != nil {
		m.factories[BusRedis] = func() Bus { return &RedisBus{Client: client, Channel: DefaultBusChannel, Log: log} }
	}
	return m
}

// Select returns the bus of kind.
func (m *BusManager) Select(kind BusKind) (Bus, error) {
	f, ok := m.factories[kind]
	if !ok {
		return nil, fmt.Errorf("events: unknown or unavailable bus %q", kind)
	}
	return f(), nil
}
