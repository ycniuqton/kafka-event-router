package httpserver

import (
	"context"
	"net"
	"net/http"
)

// drainKey stores the server's drain context in every request's base context.
type drainKey struct{}

// Drain is cancelled when a Server begins graceful shutdown. Shutdown does not cancel request contexts, so
// long-lived responses (server-sent events, long polls) watch it to end before the shutdown timeout.
type Drain struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// NewDrain returns a drain that is still open.
func NewDrain() *Drain {
	ctx, cancel := context.WithCancel(context.Background())
	return &Drain{ctx: ctx, cancel: cancel}
}

// Install makes srv carry the drain in request contexts and close it when Shutdown starts.
func (d *Drain) Install(srv *http.Server) {
	srv.BaseContext = func(net.Listener) context.Context { return context.WithValue(context.Background(), drainKey{}, d.ctx) }
	srv.RegisterOnShutdown(d.cancel)
}

// StreamContext derives the context of a long-lived response: it ends with the request or when the server drains.
type StreamContext struct{}

// Of returns ctx bound to the server drain (unchanged when the request was not served by a Server).
func (StreamContext) Of(ctx context.Context) (context.Context, context.CancelFunc) {
	out, cancel := context.WithCancel(ctx)
	drain, ok := ctx.Value(drainKey{}).(context.Context)
	if !ok {
		return out, cancel
	}
	stop := context.AfterFunc(drain, cancel)
	return out, func() {
		stop()
		cancel()
	}
}
