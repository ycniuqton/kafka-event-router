package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/preview"
)

// Deps are the collaborators of the public preview handlers.
type Deps struct {
	Client     *PreviewClient
	Authorizer PreviewAuthorizer
	Auditor    SampleAuditor
	Responder  *api.Responder
	Limiter    *PreviewRateLimiter
	TestTopics TestTopicAuditor
}

// RunBodyLimit allows inline events and large drafts.
const RunBodyLimit = api.BodyLimitLarge + api.BodyLimitDefault

// NewModule returns the public /api/v1/preview handlers.
func NewModule(d Deps) api.Module {
	return api.Modules{
		LoadSamplesHandler{d},
		ForwardHandler{d: d, spec: api.RouteSpec{Method: http.MethodGet, Pattern: preview.PrefixPublic.Pattern(preview.EndpointSessionEvents), Permission: PermSampleRead}},
		ForwardHandler{d: d, spec: api.RouteSpec{Method: http.MethodGet, Pattern: preview.PrefixPublic.Pattern(preview.EndpointSessionSchema), Permission: PermSampleRead}},
		ForwardHandler{d: d, spec: api.RouteSpec{Method: http.MethodDelete, Pattern: preview.PrefixPublic.Pattern(preview.EndpointSession), Permission: PermSampleRead}},
		ForwardHandler{d: d, spec: api.RouteSpec{Method: http.MethodPatch, Pattern: preview.PrefixPublic.Pattern(preview.EndpointSession), Permission: PermSampleRead, BodyLimit: api.BodyLimitDefault}},
		RunHandler{d: d, spec: api.RouteSpec{Method: http.MethodPost, Pattern: preview.PrefixPublic.Pattern(preview.EndpointRun), Permission: PermPreviewRead, BodyLimit: RunBodyLimit}},
		RunHandler{d: d, test: true, spec: api.RouteSpec{Method: http.MethodPost, Pattern: preview.PrefixPublic.Pattern(preview.EndpointTestRuns), Permission: PermTestRun, BodyLimit: RunBodyLimit}},
	}
}

// InternalPath maps a public preview URL onto the preview service's internal path (query kept).
type InternalPath struct{}

// Of returns the internal path of r.
func (InternalPath) Of(r *http.Request) string {
	path := string(preview.PrefixInternal) + strings.TrimPrefix(r.URL.Path, string(preview.PrefixPublic))
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	return path
}

// LoadSamplesHandler serves POST /api/v1/preview/samples (also connections' Peek with count=20).
type LoadSamplesHandler struct{ d Deps }

func (h LoadSamplesHandler) Route() api.RouteSpec {
	return api.RouteSpec{
		Method: http.MethodPost, Pattern: preview.PrefixPublic.Pattern(preview.EndpointSamples), Permission: PermSampleRead,
		Mutating: true, AuditAction: audit.ActionSampleViewed, BodyLimit: RunBodyLimit,
	}
}

// Serve rate-limits and authorizes the load, decides masking, forwards it and audits the new session.
func (h LoadSamplesHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	req, err := api.Decoder[preview.LoadRequest]{Limit: h.Route().BodyLimit}.Decode(w, r)
	if err != nil {
		return err
	}
	p := auth.PrincipalFrom(r.Context())
	if err := h.d.Limiter.Check(r.Context(), p); err != nil {
		return err
	}
	if err := h.d.Authorizer.CheckLoad(r.Context(), p, req); err != nil {
		return err
	}
	caller, err := h.d.Authorizer.Caller(r.Context(), p, req.ConnectionID)
	if err != nil {
		return err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	up, err := h.d.Client.Forward(r.Context(), r.Method, InternalPath{}.Of(r), body, caller)
	if err != nil {
		return err
	}
	if up.OK() {
		if err := h.d.Auditor.Record(r.Context(), p, req, up.Body); err != nil {
			return err
		}
	}
	return up.Write(w)
}

// ForwardHandler relays one session endpoint as-is; masking follows the session's load decision.
type ForwardHandler struct {
	d    Deps
	spec api.RouteSpec
}

func (h ForwardHandler) Route() api.RouteSpec { return h.spec }

// Serve forwards the request (bounded body when the route takes one) and relays the response.
func (h ForwardHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	var body []byte
	if h.spec.BodyLimit > 0 {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(h.spec.BodyLimit)))
		if err != nil {
			return api.JSONErrorMapper{}.Map(err)
		}
		body = raw
	}
	caller, err := h.d.Authorizer.Caller(r.Context(), auth.PrincipalFrom(r.Context()), "")
	if err != nil {
		return err
	}
	up, err := h.d.Client.Forward(r.Context(), r.Method, InternalPath{}.Of(r), body, caller)
	if err != nil {
		return err
	}
	return up.Write(w)
}

// RunHandler relays preview and test runs: rate limit, masking by the sample source (the session's
// connection, or the draft source for inline runs without a session), and for test runs writing to a
// test topic, topic.create on the destination connection plus an audit entry.
type RunHandler struct {
	d    Deps
	spec api.RouteSpec
	test bool
}

func (h RunHandler) Route() api.RouteSpec { return h.spec }

// Serve forwards the bounded body verbatim and relays the response.
func (h RunHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(h.spec.BodyLimit)))
	if err != nil {
		return api.JSONErrorMapper{}.Map(err)
	}
	p := auth.PrincipalFrom(r.Context())
	if err := h.d.Limiter.Check(r.Context(), p); err != nil {
		return err
	}
	probe := RunProbe{}.Of(body)
	if err := h.checkTestTopic(r, p, probe); err != nil {
		return err
	}
	source, err := RunSourceResolver{Client: h.d.Client, Authorizer: h.d.Authorizer}.Of(r.Context(), p, probe)
	if err != nil || source.Failure != nil {
		return source.Relay(w, err)
	}
	caller, err := h.d.Authorizer.Caller(r.Context(), p, source.ConnectionID)
	if err != nil {
		return err
	}
	up, err := h.d.Client.Forward(r.Context(), r.Method, InternalPath{}.Of(r), body, caller)
	if err != nil {
		return err
	}
	if up.OK() && h.test && probe.WriteToTopic {
		if err := h.d.TestTopics.Record(r.Context(), api.AuditContext(r), up.Body); err != nil {
			return err
		}
	}
	return up.Write(w)
}

// checkTestTopic authorizes a test-topic write on the destination connection.
func (h RunHandler) checkTestTopic(r *http.Request, p auth.Principal, probe RunProbe) error {
	if !h.test || !probe.WriteToTopic {
		return nil
	}
	return h.d.Authorizer.CheckTestTopic(r.Context(), p, probe.Refs.Destination())
}

// RunProbe is what the gateway reads from a run body without validating it.
type RunProbe struct {
	WriteToTopic bool
	Refs         preview.DraftRefs
	Session      preview.SessionID
}

// Of reads the probe from a raw run body (unreadable bodies are left to the preview service).
func (RunProbe) Of(body []byte) RunProbe {
	var req preview.RunRequest
	if json.Unmarshal(body, &req) != nil {
		return RunProbe{}
	}
	return RunProbe{WriteToTopic: req.WriteToTopic, Refs: preview.DraftRefReader{}.Of(req.Config), Session: req.MaskingSession()}
}
