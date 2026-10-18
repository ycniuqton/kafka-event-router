package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/preview"
)

// Deps are the collaborators shared by the internal handlers.
type Deps struct {
	Service   *preview.Service
	Responder *api.Responder
	Gate      *RunGate
}

// NewModule returns every internal preview handler.
func NewModule(d Deps) api.Module {
	return api.Modules{
		LoadSamplesHandler{d}, ListEventsHandler{d}, SchemaHandler{d}, SessionSourceHandler{d}, DeleteSessionHandler{d}, FormatHandler{d},
		RunHandler{d: d, mode: preview.RunPreview, endpoint: preview.EndpointRun},
		RunHandler{d: d, mode: preview.RunTest, endpoint: preview.EndpointTestRuns},
	}
}

// InternalRoutes builds route specs of internal endpoints (the caller comes from the directive).
type InternalRoutes struct{}

// Spec returns the spec of endpoint e.
func (InternalRoutes) Spec(method string, e preview.Endpoint, limit api.BodyLimit) api.RouteSpec {
	return api.RouteSpec{Method: method, Pattern: preview.PrefixInternal.Pattern(e), Auth: api.AuthNone, BodyLimit: limit}
}

// LoadSamplesHandler serves POST /internal/v1/preview/samples.
type LoadSamplesHandler struct{ d Deps }

func (h LoadSamplesHandler) Route() api.RouteSpec {
	return InternalRoutes{}.Spec(http.MethodPost, preview.EndpointSamples, api.BodyLimitLarge+api.BodyLimitDefault)
}

func (h LoadSamplesHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	req, err := api.Decoder[preview.LoadRequest]{Limit: h.Route().BodyLimit}.Decode(w, r)
	if err != nil {
		return err
	}
	res, err := h.d.Service.Load(r.Context(), CallerFrom(r.Context()), req)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusCreated, res)
}

// ListEventsHandler serves GET /internal/v1/preview/sessions/{id}/events.
type ListEventsHandler struct{ d Deps }

func (h ListEventsHandler) Route() api.RouteSpec {
	return InternalRoutes{}.Spec(http.MethodGet, preview.EndpointSessionEvents, 0)
}

func (h ListEventsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	page, err := PageReader{}.Read(r)
	if err != nil {
		return err
	}
	res, err := h.d.Service.Events(r.Context(), CallerFrom(r.Context()), SessionParam{}.Of(r), page)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, res)
}

// SchemaHandler serves GET /internal/v1/preview/sessions/{id}/schema.
type SchemaHandler struct{ d Deps }

func (h SchemaHandler) Route() api.RouteSpec {
	return InternalRoutes{}.Spec(http.MethodGet, preview.EndpointSessionSchema, 0)
}

func (h SchemaHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	res, err := h.d.Service.Schema(r.Context(), CallerFrom(r.Context()), SessionParam{}.Of(r))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, res)
}

// SessionSourceHandler serves GET /internal/v1/preview/sessions/{id} (the gateway's run masking source).
type SessionSourceHandler struct{ d Deps }

func (h SessionSourceHandler) Route() api.RouteSpec {
	return InternalRoutes{}.Spec(http.MethodGet, preview.EndpointSession, 0)
}

func (h SessionSourceHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	res, err := h.d.Service.Source(r.Context(), CallerFrom(r.Context()), SessionParam{}.Of(r))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, res)
}

// DeleteSessionHandler serves DELETE /internal/v1/preview/sessions/{id}.
type DeleteSessionHandler struct{ d Deps }

func (h DeleteSessionHandler) Route() api.RouteSpec {
	return InternalRoutes{}.Spec(http.MethodDelete, preview.EndpointSession, 0)
}

func (h DeleteSessionHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	if err := h.d.Service.Delete(r.Context(), CallerFrom(r.Context()), SessionParam{}.Of(r)); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// FormatHandler serves PATCH /internal/v1/preview/sessions/{id} (per-session format override).
type FormatHandler struct{ d Deps }

func (h FormatHandler) Route() api.RouteSpec {
	return InternalRoutes{}.Spec(http.MethodPatch, preview.EndpointSession, api.BodyLimitDefault)
}

func (h FormatHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	req, err := api.Decoder[preview.FormatRequest]{Limit: h.Route().BodyLimit}.Decode(w, r)
	if err != nil {
		return err
	}
	res, err := h.d.Service.Reformat(r.Context(), CallerFrom(r.Context()), SessionParam{}.Of(r), req)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, res)
}

// RunHandler serves POST /internal/v1/preview/run and /test-runs behind the RunGate.
type RunHandler struct {
	d        Deps
	mode     preview.RunMode
	endpoint preview.Endpoint
}

func (h RunHandler) Route() api.RouteSpec {
	return InternalRoutes{}.Spec(http.MethodPost, h.endpoint, api.BodyLimitLarge+api.BodyLimitDefault)
}

func (h RunHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	req, err := api.Decoder[preview.RunRequest]{Limit: h.Route().BodyLimit}.Decode(w, r)
	if err != nil {
		return err
	}
	if !h.d.Gate.TryAcquire() {
		return preview.ErrBusy
	}
	defer h.d.Gate.Release()
	res, err := h.d.Service.Run(r.Context(), CallerFrom(r.Context()), req, h.mode)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, res)
}

// SessionParam reads the session id route parameter.
type SessionParam struct{}

// Of returns the {id} parameter.
func (SessionParam) Of(r *http.Request) preview.SessionID {
	return preview.SessionID(chi.URLParam(r, string(preview.ParamSessionID)))
}

// PageReader parses ?cursor=&limit=&q=.
type PageReader struct{}

// Read returns the page; a bad limit is a 400 problem at /limit.
func (PageReader) Read(r *http.Request) (preview.Page, error) {
	q := r.URL.Query()
	page := preview.Page{Cursor: preview.Cursor(q.Get(string(preview.QueryCursor))), Query: q.Get(string(preview.QueryText))}
	raw := q.Get(string(preview.QueryLimit))
	if raw == "" {
		return page, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return page, preview.ErrBadLimit.At("/" + string(preview.QueryLimit))
	}
	page.Limit = n
	return page, nil
}
