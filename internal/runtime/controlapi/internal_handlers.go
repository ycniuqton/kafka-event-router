package controlapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/internal/runtime/changes"
	"github.com/eventrouter/router/internal/runtime/domain"
)

// InternalDeps are the collaborators of the internal handlers.
type InternalDeps struct {
	Workers   *WorkerAPI
	Halts     *HaltService
	Changes   *changes.Broker
	Responder *api.Responder
}

// NewInternalModule returns the /internal/v1 worker endpoints (workload identity required).
func NewInternalModule(d InternalDeps) api.Module {
	return api.Modules{&HeartbeatHandler{d: d}, &AssignmentsHandler{d: d}, &VersionConfigHandler{d: d}, &StatusHandler{d: d},
		&HaltHandler{d: d}, &ChangesHandler{d: d}}
}

// PathParams reads internal path parameters.
type PathParams struct{}

// ID returns the {id} parameter.
func (PathParams) ID(r *http.Request) string { return chi.URLParam(r, string(domain.PathParam)) }

// HeartbeatHandler serves POST /internal/v1/workers/{id}/heartbeat.
type HeartbeatHandler struct{ d InternalDeps }

func (h *HeartbeatHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(domain.PathHeartbeat), Auth: api.AuthRequired}
}

func (h *HeartbeatHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	hb, err := api.Decoder[domain.Heartbeat]{}.Decode(w, r)
	if err != nil {
		return err
	}
	reply, err := h.d.Workers.Heartbeat(r.Context(), PathParams{}.ID(r), hb)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, reply)
}

// AssignmentsHandler serves GET /internal/v1/workers/{id}/assignments with If-None-Match.
type AssignmentsHandler struct{ d InternalDeps }

func (h *AssignmentsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(domain.PathAssignments), Auth: api.AuthRequired}
}

func (h *AssignmentsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	set, err := h.d.Workers.Assignments(r.Context(), PathParams{}.ID(r))
	if err != nil {
		return err
	}
	w.Header().Set(string(domain.HeaderETag), set.ETag)
	if r.Header.Get(string(domain.HeaderIfNoneMatch)) == set.ETag {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}
	return h.d.Responder.JSON(w, http.StatusOK, set)
}

// VersionConfigHandler serves GET /internal/v1/route-versions/{id}/config (immutable).
type VersionConfigHandler struct{ d InternalDeps }

func (h *VersionConfigHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(domain.PathVersionConfig), Auth: api.AuthRequired}
}

func (h *VersionConfigHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	doc, err := h.d.Workers.VersionConfig(r.Context(), PathParams{}.ID(r))
	if err != nil {
		return err
	}
	w.Header().Set(string(HeaderContentType), string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(doc)
	return err
}

// StatusHandler serves POST /internal/v1/workers/{id}/status.
type StatusHandler struct{ d InternalDeps }

func (h *StatusHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(domain.PathStatus), Auth: api.AuthRequired, BodyLimit: api.BodyLimitLarge}
}

func (h *StatusHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	report, err := api.Decoder[domain.StatusReport]{Limit: api.BodyLimitLarge}.Decode(w, r)
	if err != nil {
		return err
	}
	if err := h.d.Workers.ReportStatus(r.Context(), PathParams{}.ID(r), report.Replicas); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// HaltHandler serves POST /internal/v1/routes/{id}/halt.
type HaltHandler struct{ d InternalDeps }

func (h *HaltHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(domain.PathHalt), Auth: api.AuthRequired}
}

func (h *HaltHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	report, err := api.Decoder[domain.HaltReport]{}.Decode(w, r)
	if err != nil {
		return err
	}
	report.RouteID = PathParams{}.ID(r)
	if err := h.d.Halts.Halt(r.Context(), report); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}

// ChangesHandler streams router_runtime change hints as server-sent events.
type ChangesHandler struct{ d InternalDeps }

func (h *ChangesHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(domain.PathChanges), Auth: api.AuthRequired}
}

func (h *ChangesHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	hints, cancel := h.d.Changes.Subscribe()
	defer cancel()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set(string(HeaderContentType), string(MediaEventStream))
	w.WriteHeader(http.StatusOK)
	SSEWriter{W: w}.Comment()
	ctx, stop := httpserver.StreamContext{}.Of(r.Context())
	defer stop()
	return SSEWriter{W: w}.Stream(ctx, hints, domain.StreamKeepAlive.D())
}

// MediaType names response media types.
type MediaType string

const (
	MediaJSON        MediaType = "application/json"
	MediaEventStream MediaType = "text/event-stream"
)

// HeaderName names response headers.
type HeaderName string

const HeaderContentType HeaderName = "Content-Type"

// SSEFrame holds the server-sent-events framing.
type SSEFrame string

const (
	SSEDataFrame    SSEFrame = "data: %s\n\n"
	SSEKeepAliveMsg SSEFrame = ": keep-alive\n\n"
)

// SSEWriter writes server-sent events and flushes after each.
type SSEWriter struct {
	W http.ResponseWriter
}

// Stream writes hints until ctx ends, with keep-alive comments.
func (s SSEWriter) Stream(ctx context.Context, hints <-chan domain.ChangeHint, keepAlive time.Duration) error {
	t := time.NewTicker(keepAlive)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			s.Comment()
		case h, ok := <-hints:
			if !ok {
				return nil
			}
			s.Event(h)
		}
	}
}

// Event writes one hint.
func (s SSEWriter) Event(h domain.ChangeHint) {
	raw, err := json.Marshal(h)
	if err != nil {
		return
	}
	fmt.Fprintf(s.W, string(SSEDataFrame), raw)
	s.flush()
}

// Comment writes a keep-alive comment.
func (s SSEWriter) Comment() {
	fmt.Fprint(s.W, string(SSEKeepAliveMsg))
	s.flush()
}

func (s SSEWriter) flush() {
	_ = http.NewResponseController(s.W).Flush()
}
