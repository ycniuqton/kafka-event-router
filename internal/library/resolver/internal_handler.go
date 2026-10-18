package resolver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/pkg/config"
)

// PinDetail is the problem detail of pin requests that cannot be served.
type PinDetail string

const (
	DetailPinMissing PinDetail = "This library version does not exist."
	DetailPinInvalid PinDetail = "Unknown library kind or invalid version."
)

// pinErrorKinds maps resolver failures to problem kinds; others pass through.
var pinErrorKinds = map[error]struct {
	kind   problem.Kind
	detail PinDetail
}{
	config.ErrLibraryRefMissing: {problem.KindNotFound, DetailPinMissing},
	ErrUnknownPinKind:           {problem.KindNotFound, DetailPinInvalid},
}

// NewInternalModule serves pinned library versions to workloads (workers compile library steps and
// named lookups through it); versions are immutable.
func NewInternalModule(r *StoreLibraryResolver, responder *api.Responder) api.Module {
	return api.Modules{&PinHandler{resolver: r, responder: responder}}
}

// PinHandler serves GET /internal/v1/library/{kind}/{name}/versions/{version} as the raw content.
type PinHandler struct {
	resolver  *StoreLibraryResolver
	responder *api.Responder
}

// Route implements api.Handler.
func (h *PinHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathPin), Auth: api.AuthRequired}
}

// Serve implements api.Handler.
func (h *PinHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	version, err := strconv.Atoi(chi.URLParam(r, string(ParamPinVersion)))
	if err != nil {
		return problem.NewError(problem.KindNotFound, string(DetailPinInvalid))
	}
	pin := Pin{Kind: PinKind(chi.URLParam(r, string(ParamPinKind))), Name: chi.URLParam(r, string(ParamPinName)), Version: version}
	raw, err := h.resolver.Content(r.Context(), pin)
	if err != nil {
		return h.translate(err)
	}
	return h.responder.JSON(w, http.StatusOK, raw)
}

// translate turns known resolver failures into problems.
func (h *PinHandler) translate(err error) error {
	for target, p := range pinErrorKinds {
		if errors.Is(err, target) {
			return problem.NewError(p.kind, string(p.detail))
		}
	}
	return err
}
