package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/problem"
)

// WSHandler is the base of every WebSocket endpoint (WS /events, live tail, …). It is mounted through
// NewWSEndpoint, so the normal chain (authenticate, rate limit, authorize, scopes) runs before the upgrade.
type WSHandler interface {
	Route() RouteSpec
	Stream(ctx context.Context, conn *WSConn) error
}

// WSReader is optionally implemented by handlers that read client messages; all others get a background
// reader that answers pings and ends the stream context when the client goes away.
type WSReader interface {
	ReadsMessages() bool
}

// WSDefaults groups the WebSocket defaults.
type WSDefaults time.Duration

const (
	DefaultWSPingInterval WSDefaults = WSDefaults(30 * time.Second)
	DefaultWSWriteTimeout WSDefaults = WSDefaults(10 * time.Second)
)

// WSReadLimit caps one inbound message in bytes.
type WSReadLimit int64

const DefaultWSReadLimit WSReadLimit = 64 << 10

// WSHeader names headers read during the upgrade.
type WSHeader string

const HeaderOrigin WSHeader = "Origin"

// ErrWSOrigin rejects an upgrade from an origin that is not allowed (cross-site WebSocket hijacking guard).
var ErrWSOrigin = problem.NewError(problem.KindForbidden, "WebSocket origin not allowed. Open the app from its configured URL and retry.")

// WSOriginPolicy is the CSRF guard of upgrades: browsers always send Origin, so a cookie-authenticated upgrade
// must come from an allowed origin (or the same host); token/workload clients without Origin are accepted.
type WSOriginPolicy struct {
	allowed []string
}

// NewWSOriginPolicy builds the policy from absolute origins (e.g. https://router.example.com).
func NewWSOriginPolicy(allowed []string) WSOriginPolicy {
	norm := make([]string, 0, len(allowed))
	for _, o := range allowed {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			norm = append(norm, strings.ToLower(u.Scheme+"://"+u.Host))
		}
	}
	return WSOriginPolicy{allowed: norm}
}

// Check validates the Origin header of r for the request principal.
func (p WSOriginPolicy) Check(r *http.Request) error {
	origin := r.Header.Get(string(HeaderOrigin))
	if origin == "" {
		if auth.PrincipalFrom(r.Context()).SessionID != "" {
			return ErrWSOrigin
		}
		return nil
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return ErrWSOrigin
	}
	if strings.EqualFold(u.Host, r.Host) || slices.Contains(p.allowed, strings.ToLower(u.Scheme+"://"+u.Host)) {
		return nil
	}
	return ErrWSOrigin
}

// WSUpgrader turns WSHandlers into api.Handlers with origin checks and ping/pong keep-alive.
type WSUpgrader struct {
	origins   WSOriginPolicy
	ping      time.Duration
	responder *Responder
}

// NewWSUpgrader builds the upgrader; ping <= 0 uses the default interval.
func NewWSUpgrader(origins WSOriginPolicy, ping time.Duration, responder *Responder) *WSUpgrader {
	if ping <= 0 {
		ping = time.Duration(DefaultWSPingInterval)
	}
	return &WSUpgrader{origins: origins, ping: ping, responder: responder}
}

// Endpoint adapts h into a mountable Handler.
func (u *WSUpgrader) Endpoint(h WSHandler) Handler {
	return &WSEndpoint{upgrader: u, handler: h}
}

// Modules adapts several WebSocket handlers into one module.
func (u *WSUpgrader) Modules(hs ...WSHandler) Module {
	out := Modules{}
	for _, h := range hs {
		out = append(out, u.Endpoint(h))
	}
	return out
}

// WSEndpoint is the api.Handler of one WebSocket route.
type WSEndpoint struct {
	upgrader *WSUpgrader
	handler  WSHandler
}

// Route implements Handler.
func (e *WSEndpoint) Route() RouteSpec {
	return e.handler.Route()
}

// Serve checks the origin, upgrades, keeps the connection alive with pings and runs the handler's stream.
func (e *WSEndpoint) Serve(w http.ResponseWriter, r *http.Request) error {
	if err := e.upgrader.origins.Check(r); err != nil {
		return err
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return nil
	}
	c.SetReadLimit(int64(DefaultWSReadLimit))
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if reader, ok := e.handler.(WSReader); !ok || !reader.ReadsMessages() {
		ctx = c.CloseRead(ctx)
	}
	conn := &WSConn{c: c, principal: auth.PrincipalFrom(r.Context())}
	go conn.keepAlive(ctx, cancel, e.upgrader.ping)
	conn.finish(e.handler.Stream(ctx, conn))
	return nil
}

// WSCloseReason names close reasons sent to clients.
type WSCloseReason string

const (
	CloseNormal   WSCloseReason = "bye"
	CloseInternal WSCloseReason = "server error"
)

// WSConn is the connection handed to a WSHandler: JSON messages, the authenticated principal, close.
type WSConn struct {
	c         *websocket.Conn
	principal auth.Principal
}

// Principal returns the caller authenticated before the upgrade.
func (c *WSConn) Principal() auth.Principal {
	return c.principal
}

// WriteJSON sends one JSON text message (bounded by the default write timeout).
func (c *WSConn) WriteJSON(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(DefaultWSWriteTimeout))
	defer cancel()
	return c.c.Write(ctx, websocket.MessageText, data)
}

// ReadJSON blocks for the next message and decodes it into v.
func (c *WSConn) ReadJSON(ctx context.Context, v any) error {
	_, data, err := c.c.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// keepAlive pings every interval; a missed pong cancels the stream.
func (c *WSConn) keepAlive(ctx context.Context, cancel context.CancelFunc, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pctx, pcancel := context.WithTimeout(ctx, interval)
			err := c.c.Ping(pctx)
			pcancel()
			if err != nil {
				cancel()
				return
			}
		}
	}
}

// WSCloseError ends a stream with an application close code and reason (e.g. 4409 "route not running").
type WSCloseError struct {
	Code   int
	Reason string
}

func (e *WSCloseError) Error() string { return e.Reason }

// finish closes the connection with a status that reflects how the stream ended.
func (c *WSConn) finish(err error) {
	var closeErr *WSCloseError
	if errors.As(err, &closeErr) {
		_ = c.c.Close(websocket.StatusCode(closeErr.Code), closeErr.Reason)
		return
	}
	if err == nil || errors.Is(err, context.Canceled) || websocket.CloseStatus(err) != -1 {
		_ = c.c.Close(websocket.StatusNormalClosure, string(CloseNormal))
		return
	}
	_ = c.c.Close(websocket.StatusInternalError, string(CloseInternal))
}
