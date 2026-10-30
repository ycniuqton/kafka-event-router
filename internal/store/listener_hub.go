package store

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
)

// NoticeKind tells a subscriber whether a notice carries a payload or asks it to resync.
type NoticeKind string

const (
	NoticeMessage NoticeKind = "message"
	NoticeReset   NoticeKind = "reset"
)

// Notice is what a subscriber receives: one NOTIFY payload, or a reset (notifications may have been missed).
type Notice struct {
	Kind    NoticeKind
	Payload string
}

// NotificationSource is the surface LISTEN consumers depend on (the process ListenerHub, or a test fake).
type NotificationSource interface {
	Subscribe(ch Channel) *Subscription
}

// ListenerTimings bounds the hub's reconnect backoff and idle keepalive.
type ListenerTimings struct {
	MinBackoff time.Duration
	MaxBackoff time.Duration
	Keepalive  time.Duration
	Ping       time.Duration
}

// ListenerTiming names the default durations of the listener session.
type ListenerTiming time.Duration

const (
	ListenMinBackoff ListenerTiming = ListenerTiming(500 * time.Millisecond)
	ListenMaxBackoff ListenerTiming = ListenerTiming(30 * time.Second)
	ListenKeepalive  ListenerTiming = ListenerTiming(30 * time.Second)
	ListenPing       ListenerTiming = ListenerTiming(5 * time.Second)
)

// DefaultListenerTimings returns the production timings.
func DefaultListenerTimings() ListenerTimings {
	return ListenerTimings{MinBackoff: time.Duration(ListenMinBackoff), MaxBackoff: time.Duration(ListenMaxBackoff),
		Keepalive: time.Duration(ListenKeepalive), Ping: time.Duration(ListenPing)}
}

// ListenerBuffer sizes the per-subscription notice buffer.
type ListenerBuffer int

const SubscriptionBuffer ListenerBuffer = 256

// ListenerHub multiplexes every LISTEN of a process onto one dedicated session (never a pooled connection),
// fans notifications out per channel, and reconnects with backoff, re-LISTENing and resetting subscribers.
type ListenerHub struct {
	dialer   ListenDialer
	timings  ListenerTimings
	log      *slog.Logger
	mu       sync.Mutex
	subs     map[Channel]map[*Subscription]struct{}
	listened map[Channel]bool
	dirty    bool
	wake     context.CancelFunc
	start    sync.Once
	ctx      context.Context
	stop     context.CancelFunc
	done     chan struct{}
}

// NewListenerHub builds an idle hub; the session starts with the first subscription.
func NewListenerHub(dialer ListenDialer, timings ListenerTimings, log *slog.Logger) *ListenerHub {
	ctx, stop := context.WithCancel(context.Background())
	if log == nil {
		log = slog.Default()
	}
	return &ListenerHub{dialer: dialer, timings: timings, log: log, subs: map[Channel]map[*Subscription]struct{}{},
		listened: map[Channel]bool{}, ctx: ctx, stop: stop, done: make(chan struct{})}
}

// Subscribe registers a subscriber of ch; it receives a reset once ch is LISTENed, then every notification.
func (h *ListenerHub) Subscribe(ch Channel) *Subscription {
	s := &Subscription{hub: h, channel: ch, ch: make(chan Notice, SubscriptionBuffer)}
	h.mu.Lock()
	if h.subs[ch] == nil {
		h.subs[ch] = map[*Subscription]struct{}{}
	}
	h.subs[ch][s] = struct{}{}
	if h.listened[ch] {
		s.offer(Notice{Kind: NoticeReset})
	} else {
		h.markDirtyLocked()
	}
	h.mu.Unlock()
	h.start.Do(func() { go h.run() })
	return s
}

// Close stops the session and closes every subscription.
func (h *ListenerHub) Close() {
	h.stop()
	h.start.Do(func() { close(h.done) })
	<-h.done
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.subs {
		for s := range set {
			close(s.ch)
		}
	}
	h.subs = map[Channel]map[*Subscription]struct{}{}
}

// run keeps one session alive until Close, backing off between failed attempts.
func (h *ListenerHub) run() {
	defer close(h.done)
	backoff := h.timings.MinBackoff
	for h.ctx.Err() == nil {
		established, err := h.session()
		if h.ctx.Err() != nil {
			return
		}
		if established {
			backoff = h.timings.MinBackoff
		}
		h.log.Warn("store: listener session lost; reconnecting", string(logging.AttrError), err)
		select {
		case <-h.ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, h.timings.MaxBackoff)
	}
}

// session dials, LISTENs every subscribed channel and relays notifications until the session fails.
func (h *ListenerHub) session() (bool, error) {
	conn, err := h.dialer.Dial(h.ctx)
	if err != nil {
		return false, err
	}
	defer h.closeConn(conn)
	h.forgetListened()
	if err := h.syncChannels(conn); err != nil {
		return false, err
	}
	for {
		if err := h.waitOnce(conn); err != nil {
			return true, err
		}
	}
}

// waitOnce waits for one notification, a wake-up (new channel) or the keepalive tick.
func (h *ListenerHub) waitOnce(conn ListenConn) error {
	waitCtx, cancel := context.WithTimeout(h.ctx, h.timings.Keepalive)
	defer cancel()
	h.armWake(cancel)
	n, err := conn.WaitForNotification(waitCtx)
	h.disarmWake()
	switch {
	case err == nil:
		h.deliver(n)
		return nil
	case h.ctx.Err() != nil:
		return h.ctx.Err()
	case waitCtx.Err() == nil:
		return err
	}
	return h.idle(conn, waitCtx.Err())
}

// idle LISTENs newly subscribed channels and, on a keepalive tick, pings the session.
func (h *ListenerHub) idle(conn ListenConn, cause error) error {
	if err := h.syncChannels(conn); err != nil {
		return err
	}
	if !errors.Is(cause, context.DeadlineExceeded) {
		return nil
	}
	ctx, cancel := context.WithTimeout(h.ctx, h.timings.Ping)
	defer cancel()
	return conn.Ping(ctx)
}

// syncChannels LISTENs every subscribed channel not yet LISTENed on this session and resets its subscribers.
func (h *ListenerHub) syncChannels(conn ListenConn) error {
	for _, ch := range h.pendingChannels() {
		if err := conn.Listen(h.ctx, ch); err != nil {
			return err
		}
		h.markListened(ch)
	}
	return nil
}

// pendingChannels returns the subscribed channels the session does not LISTEN yet.
func (h *ListenerHub) pendingChannels() []Channel {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dirty = false
	out := []Channel{}
	for ch, set := range h.subs {
		if len(set) > 0 && !h.listened[ch] {
			out = append(out, ch)
		}
	}
	return out
}

// markListened records ch as live and resets its subscribers.
func (h *ListenerHub) markListened(ch Channel) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.listened[ch] = true
	for s := range h.subs[ch] {
		s.offer(Notice{Kind: NoticeReset})
	}
}

// deliver fans one notification out to the subscribers of its channel.
func (h *ListenerHub) deliver(n Notification) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs[n.Channel] {
		s.offer(Notice{Kind: NoticeMessage, Payload: n.Payload})
	}
}

// forgetListened clears the LISTEN set of the previous session.
func (h *ListenerHub) forgetListened() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.listened = map[Channel]bool{}
}

// armWake lets a new subscription interrupt the current wait; a pending change interrupts it at once.
func (h *ListenerHub) armWake(cancel context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.wake = cancel
	if h.dirty {
		cancel()
	}
}

func (h *ListenerHub) disarmWake() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.wake = nil
}

// markDirtyLocked flags a channel to LISTEN and interrupts the current wait (caller holds mu).
func (h *ListenerHub) markDirtyLocked() {
	h.dirty = true
	if h.wake != nil {
		h.wake()
	}
}

func (h *ListenerHub) closeConn(conn ListenConn) {
	ctx, cancel := context.WithTimeout(context.Background(), h.timings.Ping)
	defer cancel()
	_ = conn.Close(ctx)
}

// remove drops s from the hub and closes its channel.
func (h *ListenerHub) remove(s *Subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.subs[s.channel]
	if _, ok := set[s]; !ok {
		return
	}
	delete(set, s)
	close(s.ch)
}

// Subscription is one subscriber's notice stream of a channel.
type Subscription struct {
	hub     *ListenerHub
	channel Channel
	ch      chan Notice
}

// C returns the notice stream; it is closed by Close or when the hub closes.
func (s *Subscription) C() <-chan Notice { return s.ch }

// Close unsubscribes.
func (s *Subscription) Close() { s.hub.remove(s) }

// offer delivers n without blocking; a full buffer is coalesced into one reset (caller holds the hub lock).
func (s *Subscription) offer(n Notice) {
	select {
	case s.ch <- n:
		return
	default:
	}
	for len(s.ch) > 0 {
		select {
		case <-s.ch:
		default:
		}
	}
	s.ch <- Notice{Kind: NoticeReset}
}
