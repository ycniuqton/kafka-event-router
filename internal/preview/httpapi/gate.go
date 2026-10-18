// Package httpapi serves the preview service's internal endpoints (/internal/v1/preview/*):
// class handlers, the caller-directive stage and the run concurrency gate.
package httpapi

import (
	"context"
	"net/http"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/preview"
)

// RunGate bounds concurrent preview runs per instance; a full gate answers 429.
type RunGate struct {
	slots chan struct{}
}

// NewRunGate builds a gate with n slots.
func NewRunGate(n int) *RunGate {
	return &RunGate{slots: make(chan struct{}, max(n, 1))}
}

// TryAcquire takes a slot without waiting.
func (g *RunGate) TryAcquire() bool {
	select {
	case g.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release frees a slot.
func (g *RunGate) Release() {
	<-g.slots
}

type callerKey struct{}

// CallerFrom returns the verified caller of the request.
func CallerFrom(ctx context.Context) preview.Caller {
	c, _ := ctx.Value(callerKey{}).(preview.Caller)
	return c
}

// DirectiveStage verifies the signed X-Router-Caller header and stores the caller in the context.
type DirectiveStage struct {
	Codec     preview.DirectiveCodec
	Responder *api.Responder
}

// Wrap implements api.RouteMiddleware.
func (s DirectiveStage) Wrap(_ api.RouteSpec, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, err := s.Codec.Decode(r.Header.Get(string(preview.HeaderCaller)))
		if err != nil {
			s.Responder.Problem(w, r, preview.ErrDirective.Wrap(err))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), callerKey{}, caller)))
	})
}
