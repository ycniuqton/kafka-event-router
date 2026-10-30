package changes

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/store"
)

// scriptedConn is a LISTEN session that plays queued notifications.
type scriptedConn struct {
	notes chan store.Notification
}

func (c scriptedConn) Listen(context.Context, store.Channel) error { return nil }
func (c scriptedConn) Ping(context.Context) error                  { return nil }
func (c scriptedConn) Close(context.Context) error                 { return nil }
func (c scriptedConn) WaitForNotification(ctx context.Context) (store.Notification, error) {
	select {
	case n := <-c.notes:
		return n, nil
	case <-ctx.Done():
		return store.Notification{}, ctx.Err()
	}
}

type scriptedDialer struct{ conn scriptedConn }

func (d scriptedDialer) Dial(context.Context) (store.ListenConn, error) { return d.conn, nil }

func nextHint(t *testing.T, ch <-chan domain.ChangeHint) domain.ChangeHint {
	t.Helper()
	select {
	case h := <-ch:
		return h
	case <-time.After(2 * time.Second):
		t.Fatal("no hint")
		return domain.ChangeHint{}
	}
}

func TestPgListenerRelaysHubNoticesAsHints(t *testing.T) {
	conn := scriptedConn{notes: make(chan store.Notification, 4)}
	hub := store.NewListenerHub(scriptedDialer{conn: conn}, store.DefaultListenerTimings(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer hub.Close()
	broker := NewBroker()
	hints, cancel := broker.Subscribe()
	defer cancel()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go PgListener{Notifications: hub, Broker: broker}.Run(ctx)
	if h := nextHint(t, hints); h.Kind != domain.ChangeResync {
		t.Fatalf("first hint = %+v, want resync on listen", h)
	}
	ch := store.Channel(domain.ChannelRouterRuntime)
	conn.notes <- store.Notification{Channel: ch, Payload: "not json"}
	conn.notes <- store.Notification{Channel: ch, Payload: `{"route_id":"r1","kind":"route"}`}
	if h := nextHint(t, hints); h.RouteID != "r1" || h.Kind != domain.ChangeRoute {
		t.Fatalf("hint = %+v", h)
	}
}
