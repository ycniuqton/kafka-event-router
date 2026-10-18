package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/metrics"
	"github.com/eventrouter/router/internal/platform/problem"
)

type sessionAuth struct{}

func (sessionAuth) Authenticate(*http.Request) (auth.Principal, error) {
	return auth.Principal{Kind: auth.PrincipalUser, Email: "a@b.c", SessionID: "s1"}, nil
}

type tickStream struct{ done chan struct{} }

func (h *tickStream) Route() RouteSpec {
	return RouteSpec{Method: http.MethodGet, Pattern: "/api/v1/events"}
}

func (h *tickStream) Stream(ctx context.Context, conn *WSConn) error {
	defer close(h.done)
	if err := conn.WriteJSON(ctx, map[string]string{"hello": conn.Principal().Email}); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

func newWSServer(t *testing.T, h WSHandler) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := NewResponder(problem.NewCatalog(), log)
	collector := metrics.NewHTTPCollector(metrics.NewRegistry().Registerer())
	m := NewMounter(responder, []GlobalMiddleware{RequestIDMiddleware{}, MetricsMiddleware{Collector: collector}, LoggingMiddleware{Log: log}},
		[]RouteMiddleware{AuthenticateStage{Authenticator: sessionAuth{}, Responder: responder}})
	router := newRouter()
	m.Install(router)
	up := NewWSUpgrader(NewWSOriginPolicy([]string{"http://app.example"}), 50*time.Millisecond, responder)
	m.Mount(router, up.Modules(h))
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestWebSocketUpgradeThroughChain(t *testing.T) {
	h := &tickStream{done: make(chan struct{})}
	srv := newWSServer(t, h)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/v1/events",
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"http://app.example"}}})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, _, _ = c.Read(ctx) }()
	time.Sleep(200 * time.Millisecond)
	_ = c.Close(websocket.StatusNormalClosure, "")
	select {
	case <-h.done:
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not end after client close")
	}
}

func TestWebSocketFirstMessageAndPings(t *testing.T) {
	srv := newWSServer(t, &tickStream{done: make(chan struct{})})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/v1/events",
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"http://app.example"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	_, data, err := c.Read(ctx)
	if err != nil || string(data) != `{"hello":"a@b.c"}` {
		t.Fatalf("first message %q, %v", data, err)
	}
	readCtx, readCancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer readCancel()
	if _, _, err := c.Read(readCtx); websocket.CloseStatus(err) != -1 {
		t.Fatalf("connection closed while pings answered: %v", err)
	}
}

func TestWebSocketRejectsForeignOrigin(t *testing.T) {
	srv := newWSServer(t, &tickStream{done: make(chan struct{})})
	for _, origin := range []string{"http://evil.example", ""} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/events", nil)
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Sec-WebSocket-Version", "13")
		req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("origin %q: status %d", origin, resp.StatusCode)
		}
	}
}
