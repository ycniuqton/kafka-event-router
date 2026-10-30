package store

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventrouter/router/internal/platform/config"
)

// fakeListenConn is an in-memory LISTEN session: tests push notifications and break it.
type fakeListenConn struct {
	mu       sync.Mutex
	listened []Channel
	notes    chan Notification
	broken   chan error
	pingErr  error
	closed   bool
}

func newFakeListenConn() *fakeListenConn {
	return &fakeListenConn{notes: make(chan Notification, 16), broken: make(chan error, 1)}
}

func (c *fakeListenConn) Listen(_ context.Context, ch Channel) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listened = append(c.listened, ch)
	return nil
}

func (c *fakeListenConn) WaitForNotification(ctx context.Context) (Notification, error) {
	select {
	case n := <-c.notes:
		return n, nil
	case err := <-c.broken:
		return Notification{}, err
	case <-ctx.Done():
		return Notification{}, ctx.Err()
	}
}

func (c *fakeListenConn) Ping(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pingErr
}

func (c *fakeListenConn) Close(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *fakeListenConn) channels() []Channel {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Channel(nil), c.listened...)
}

// fakeDialer hands out sessions in order and counts dials; failFirst dials fail before the first session.
type fakeDialer struct {
	mu        sync.Mutex
	conns     chan *fakeListenConn
	failFirst int
	dials     int
}

func newFakeDialer(conns ...*fakeListenConn) *fakeDialer {
	d := &fakeDialer{conns: make(chan *fakeListenConn, 8)}
	for _, c := range conns {
		d.conns <- c
	}
	return d
}

func (d *fakeDialer) Dial(ctx context.Context) (ListenConn, error) {
	d.mu.Lock()
	d.dials++
	fail := d.dials <= d.failFirst
	d.mu.Unlock()
	if fail {
		return nil, errors.New("dial refused")
	}
	select {
	case c := <-d.conns:
		return c, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (d *fakeDialer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dials
}

func fastTimings() ListenerTimings {
	return ListenerTimings{MinBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond, Keepalive: 20 * time.Millisecond, Ping: time.Second}
}

var quietLog = slog.New(slog.NewTextHandler(io.Discard, nil))

const (
	chA Channel = "chan_a"
	chB Channel = "chan_b"
)

func expectNotice(t *testing.T, s *Subscription, want Notice) {
	t.Helper()
	select {
	case got := <-s.C():
		if got != want {
			t.Fatalf("notice = %+v, want %+v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no notice, want %+v", want)
	}
}

func expectSilence(t *testing.T, s *Subscription) {
	t.Helper()
	select {
	case got := <-s.C():
		t.Fatalf("unexpected notice %+v", got)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestListenerHubFansOutPerChannelOnOneSession(t *testing.T) {
	conn := newFakeListenConn()
	d := newFakeDialer(conn)
	hub := NewListenerHub(d, fastTimings(), quietLog)
	defer hub.Close()
	a1, a2, b := hub.Subscribe(chA), hub.Subscribe(chA), hub.Subscribe(chB)
	for _, s := range []*Subscription{a1, a2, b} {
		expectNotice(t, s, Notice{Kind: NoticeReset})
	}
	conn.notes <- Notification{Channel: chA, Payload: "x"}
	expectNotice(t, a1, Notice{Kind: NoticeMessage, Payload: "x"})
	expectNotice(t, a2, Notice{Kind: NoticeMessage, Payload: "x"})
	expectSilence(t, b)
	if d.count() != 1 || len(conn.channels()) != 2 {
		t.Fatalf("dials=%d listened=%v, want one session listening both channels once", d.count(), conn.channels())
	}
}

func TestListenerHubSubscribesNewChannelWithoutReconnect(t *testing.T) {
	conn := newFakeListenConn()
	d := newFakeDialer(conn)
	hub := NewListenerHub(d, ListenerTimings{MinBackoff: time.Millisecond, MaxBackoff: time.Millisecond, Keepalive: time.Hour, Ping: time.Second}, quietLog)
	defer hub.Close()
	a := hub.Subscribe(chA)
	expectNotice(t, a, Notice{Kind: NoticeReset})
	b := hub.Subscribe(chB)
	expectNotice(t, b, Notice{Kind: NoticeReset})
	conn.notes <- Notification{Channel: chB, Payload: "late"}
	expectNotice(t, b, Notice{Kind: NoticeMessage, Payload: "late"})
	late := hub.Subscribe(chA)
	expectNotice(t, late, Notice{Kind: NoticeReset})
	if d.count() != 1 {
		t.Fatalf("dials = %d, want 1", d.count())
	}
}

func TestListenerHubReconnectsResubscribesAndResets(t *testing.T) {
	first, second := newFakeListenConn(), newFakeListenConn()
	d := newFakeDialer(first, second)
	d.failFirst = 2
	hub := NewListenerHub(d, fastTimings(), quietLog)
	defer hub.Close()
	a, b := hub.Subscribe(chA), hub.Subscribe(chB)
	expectNotice(t, a, Notice{Kind: NoticeReset})
	expectNotice(t, b, Notice{Kind: NoticeReset})
	first.broken <- errors.New("connection reset")
	expectNotice(t, a, Notice{Kind: NoticeReset})
	expectNotice(t, b, Notice{Kind: NoticeReset})
	if got := second.channels(); len(got) != 2 {
		t.Fatalf("second session listened %v, want both channels", got)
	}
	second.notes <- Notification{Channel: chA, Payload: "after"}
	expectNotice(t, a, Notice{Kind: NoticeMessage, Payload: "after"})
}

func TestListenerHubReplacesSessionWhenKeepalivePingFails(t *testing.T) {
	first, second := newFakeListenConn(), newFakeListenConn()
	first.pingErr = errors.New("dead session")
	hub := NewListenerHub(newFakeDialer(first, second), fastTimings(), quietLog)
	defer hub.Close()
	a := hub.Subscribe(chA)
	expectNotice(t, a, Notice{Kind: NoticeReset})
	expectNotice(t, a, Notice{Kind: NoticeReset})
	second.notes <- Notification{Channel: chA, Payload: "ok"}
	expectNotice(t, a, Notice{Kind: NoticeMessage, Payload: "ok"})
}

func TestListenerHubCoalescesOverflowIntoReset(t *testing.T) {
	conn := newFakeListenConn()
	hub := NewListenerHub(newFakeDialer(conn), fastTimings(), quietLog)
	defer hub.Close()
	slow, fast := hub.Subscribe(chA), hub.Subscribe(chA)
	expectNotice(t, slow, Notice{Kind: NoticeReset})
	expectNotice(t, fast, Notice{Kind: NoticeReset})
	for i := 0; i <= int(SubscriptionBuffer); i++ {
		hub.deliver(Notification{Channel: chA, Payload: "p"})
		<-fast.C()
	}
	expectNotice(t, slow, Notice{Kind: NoticeReset})
	expectSilence(t, slow)
}

func TestListenerHubCloseEndsSubscriptions(t *testing.T) {
	conn := newFakeListenConn()
	hub := NewListenerHub(newFakeDialer(conn), fastTimings(), quietLog)
	a := hub.Subscribe(chA)
	expectNotice(t, a, Notice{Kind: NoticeReset})
	b := hub.Subscribe(chB)
	b.Close()
	hub.Close()
	if _, ok := <-a.C(); ok {
		t.Fatal("subscription must be closed with the hub")
	}
	a.Close()
	NewListenerHub(newFakeDialer(), fastTimings(), quietLog).Close()
}

func TestPoolSizingDefault(t *testing.T) {
	cases := []struct{ configured, cpus, want int }{{0, 2, 10}, {0, 8, 16}, {25, 2, 25}, {-1, 1, 10}}
	for _, c := range cases {
		if got := (PoolSizing{}).Default(c.configured, c.cpus); got != c.want {
			t.Fatalf("Default(%d, %d) = %d, want %d", c.configured, c.cpus, got, c.want)
		}
	}
	for _, c := range []struct {
		url  string
		knob int
		want int32
	}{{"postgres://u@h/db?pool_max_conns=3", 25, 3}, {"postgres://u@h/db", 25, 25}} {
		poolCfg, err := pgxpool.ParseConfig(c.url)
		if err != nil {
			t.Fatal(err)
		}
		PoolSizing{}.Apply(poolCfg, config.DatabaseConfig{URL: c.url, PoolMaxConns: c.knob})
		if poolCfg.MaxConns != c.want {
			t.Fatalf("%s knob %d: MaxConns = %d, want %d", c.url, c.knob, poolCfg.MaxConns, c.want)
		}
	}
}
