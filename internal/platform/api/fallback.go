package api

import (
	"net/http"

	"github.com/eventrouter/router/internal/platform/problem"
)

// FallbackDetail names messages of router-level fallbacks.
type FallbackDetail string

const (
	DetailNoRoute  FallbackDetail = "No such resource. Check the URL."
	DetailNoMethod FallbackDetail = "This method is not supported on this resource."
)

// NotFoundHandler answers unknown routes with a 404 problem.
type NotFoundHandler struct {
	Responder *Responder
}

func (h NotFoundHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Responder.Problem(w, r, problem.NewError(problem.KindNotFound, string(DetailNoRoute)))
}

// MethodNotAllowedHandler answers unsupported methods with a 405 problem.
type MethodNotAllowedHandler struct {
	Responder *Responder
}

func (h MethodNotAllowedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Responder.Writer().Write(w, r, problem.New(problem.TypeMethodNotAllowed, http.StatusMethodNotAllowed, string(DetailNoMethod)))
}
