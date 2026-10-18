package sandboxinfo

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// Path names the sandbox routes and params.
type Path string

const (
	PathRuntimeInfo      Path = "/api/v1/sandbox/runtime-info"
	PathTeamSalt         Path = "/internal/v1/sandbox/teams/{teamId}/hash-salt"
	ParamTeamID          Path = "teamId"
	ParamTeamPlaceholder Path = "{teamId}"
)

// PermRuntimeInfo lets every Studio reader fetch the editor's runtime info.
var PermRuntimeInfo = auth.Permission{Key: "transform.read", Access: auth.AccessRead}

// InfoSource supplies the runtime info (sandbox.Manager or a StaticInfo built from config).
type InfoSource interface {
	Info() sandbox.RuntimeInfo
}

// StaticInfo builds runtime info from config without booting an engine (the API never runs JS).
type StaticInfo struct {
	Config sandbox.Config
}

// Info implements InfoSource.
func (s StaticInfo) Info() sandbox.RuntimeInfo {
	return sandbox.InfoBuilder{}.Build(s.Config.Engine, sandbox.NewLimitPolicy(s.Config.Maxima))
}

// Deps are what the sandbox handlers need.
type Deps struct {
	Info      InfoSource
	Checker   SourceChecker
	MaxSource int64
	Salts     host.SaltProvider
	Recorder  audit.Recorder
	Responder *api.Responder
}

// NewModule returns the public module: GET runtime-info, plus POST check when a checker is set.
func NewModule(d Deps) (api.Module, error) {
	h, err := NewRuntimeInfoHandler(d.Info, d.Responder)
	if err != nil {
		return nil, err
	}
	if d.Checker == nil {
		return api.Modules{h}, nil
	}
	return api.Modules{h, NewCheckHandler(d.Checker, d.MaxSource, d.Responder)}, nil
}

// NewInternalModule returns the workload-only module (team hash salts for workers and preview).
func NewInternalModule(d Deps) api.Module {
	return api.Modules{&TeamSaltHandler{d: d}}
}

// RuntimeInfoHandler serves the static runtime info with a strong ETag; a matching If-None-Match is 304.
type RuntimeInfoHandler struct {
	body      []byte
	etag      string
	responder *api.Responder
}

// NewRuntimeInfoHandler renders the body once (the info only changes with a deploy).
func NewRuntimeInfoHandler(info InfoSource, responder *api.Responder) (*RuntimeInfoHandler, error) {
	body, err := json.Marshal(info.Info())
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	return &RuntimeInfoHandler{body: body, etag: `"` + hex.EncodeToString(sum[:]) + `"`, responder: responder}, nil
}

// Route implements api.Handler.
func (h *RuntimeInfoHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathRuntimeInfo), Permission: PermRuntimeInfo}
}

// Serve implements api.Handler.
func (h *RuntimeInfoHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set(string(api.HeaderETag), h.etag)
	if (ETagMatch{}).Matches(r.Header.Get(string(HeaderIfNoneMatch)), h.etag) {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}
	w.Header().Set(string(httpserver.HeaderContentType), string(httpserver.MediaTypeJSON))
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(h.body)
	return err
}

// TeamSaltResponse is the internal salt body; the value is base64 of the raw salt.
type TeamSaltResponse struct {
	TeamID   string `json:"teamId"`
	ValueB64 string `json:"value_b64"`
}

// TeamSaltHandler serves GET /internal/v1/sandbox/teams/{teamId}/hash-salt to workloads; the salt is
// generated on first use and every read is audited as a secret read on the team.
type TeamSaltHandler struct{ d Deps }

// Route implements api.Handler.
func (h *TeamSaltHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathTeamSalt), Auth: api.AuthRequired}
}

// Serve implements api.Handler.
func (h *TeamSaltHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	team := chi.URLParam(r, string(ParamTeamID))
	salt, err := h.d.Salts.Salt(r.Context(), team)
	if err != nil {
		return err
	}
	entry := api.AuditContext(r).Apply(audit.Entry{Action: audit.ActionSecretRead, Object: audit.ObjectTeam, ObjectID: team})
	if err := h.d.Recorder.Record(r.Context(), entry); err != nil {
		return err
	}
	w.Header().Set(string(api.HeaderCacheControl), string(api.CacheNoStore))
	return h.d.Responder.JSON(w, http.StatusOK, TeamSaltResponse{TeamID: team, ValueB64: base64.StdEncoding.EncodeToString(salt)})
}
