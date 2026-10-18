package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RouteMiddleware is one per-route stage of the chain (authenticate, CSRF, authorize, audit, …).
type RouteMiddleware interface {
	Wrap(spec RouteSpec, next http.Handler) http.Handler
}

// GlobalMiddleware is a router-level stage applied to every request of a listener.
type GlobalMiddleware interface {
	Handler(next http.Handler) http.Handler
}

// Mounter attaches handlers with the chain from design §4.1:
// RequestID → Recover → Metrics → Logging → [RateLimit] → Authenticate → CSRF → Authorize → Audit → handler.
type Mounter struct {
	responder *Responder
	global    []GlobalMiddleware
	stages    []RouteMiddleware
}

// NewMounter builds a mounter; global and stages are applied in the given order.
func NewMounter(responder *Responder, global []GlobalMiddleware, stages []RouteMiddleware) *Mounter {
	return &Mounter{responder: responder, global: global, stages: stages}
}

// Install adds the global middlewares and problem fallbacks for unknown routes/methods.
func (m *Mounter) Install(r chi.Router) {
	for _, g := range m.global {
		r.Use(g.Handler)
	}
	r.NotFound(NotFoundHandler{Responder: m.responder}.ServeHTTP)
	r.MethodNotAllowed(MethodNotAllowedHandler{Responder: m.responder}.ServeHTTP)
}

// Mount registers every handler of every module on r.
func (m *Mounter) Mount(r chi.Router, modules ...Module) {
	for _, mod := range modules {
		for _, h := range mod.Handlers() {
			spec := h.Route()
			r.Method(spec.Method, spec.Pattern, m.chain(spec, h))
		}
	}
}

// Responder exposes the shared responder.
func (m *Mounter) Responder() *Responder {
	return m.responder
}

func (m *Mounter) chain(spec RouteSpec, h Handler) http.Handler {
	var next http.Handler = ServeAdapter{handler: h, responder: m.responder}
	for i := len(m.stages) - 1; i >= 0; i-- {
		next = m.stages[i].Wrap(spec, next)
	}
	return next
}

// ServeAdapter turns Handler.Serve's error into a problem response.
type ServeAdapter struct {
	handler   Handler
	responder *Responder
}

// ServeHTTP implements http.Handler.
func (a ServeAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := a.handler.Serve(w, r); err != nil {
		a.responder.Problem(w, r, err)
	}
}
