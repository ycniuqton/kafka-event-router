// Package livetailhttp serves WS /api/v1/routes/{id}/tail: one socket per open Live tail tab.
package livetailhttp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/livetail"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/requestid"
	routeshttp "github.com/eventrouter/router/internal/routes/http"
)

// Path names the tail endpoint.
type Path string

const PathTail Path = "/api/v1/routes/{id}/tail"

// PermLiveTail is the permission of the tail (viewer of the route's team).
var PermLiveTail = auth.Permission{Key: "livetail.read", Access: auth.AccessRead}

// CloseCode is a WebSocket close code of the tail.
type CloseCode int

const (
	CloseRouteNotRunning CloseCode = 4409
	CloseUnavailable     CloseCode = 1011
)

// CloseReason is the text sent with a close code.
type CloseReason string

const (
	ReasonNotRunning  CloseReason = "route not running"
	ReasonUnavailable CloseReason = "live tail unavailable"
)

// OpenedDetail is the audit diff of livetail.opened.
type OpenedDetail struct {
	Masked bool `json:"masked"`
}

// TailAuditor records that a tail was opened (with its masked flag).
type TailAuditor interface {
	Opened(ctx context.Context, p auth.Principal, r livetail.TailRoute, masked bool) error
}

// RecorderAuditor writes livetail.opened through the audit Recorder.
type RecorderAuditor struct {
	Recorder audit.Recorder
}

// Opened implements TailAuditor.
func (a RecorderAuditor) Opened(ctx context.Context, p auth.Principal, r livetail.TailRoute, masked bool) error {
	diff, err := json.Marshal(OpenedDetail{Masked: masked})
	if err != nil {
		return err
	}
	e := audit.Entry{Actor: auth.PrincipalActor(p), Action: audit.ActionLiveTailOpened, Object: audit.ObjectRoute,
		ObjectID: r.ID, Diff: diff, ObjectLabel: r.Name, Sensitive: !masked, RequestID: requestid.From(ctx)}
	if team, err := uuid.Parse(r.TeamID); err == nil {
		e.TeamID = &team
	}
	return a.Recorder.Record(ctx, e)
}

// Deps are the collaborators of the tail endpoint.
type Deps struct {
	Routes     livetail.RouteDirectory
	Store      livetail.TailStore
	Visibility livetail.VisibilityDecider
	Masker     livetail.TailMasker
	Audit      TailAuditor
	Limits     livetail.TailLimits
	Log        *slog.Logger
}

// TailHandler streams one route's live tail (design flow 5.7).
type TailHandler struct {
	D Deps
}

// Route implements api.WSHandler.
func (h *TailHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathTail), Permission: PermLiveTail, Object: routeshttp.RouteObject}
}

// ReadsMessages implements api.WSReader: the client sends pause/resume/filter ops.
func (h *TailHandler) ReadsMessages() bool { return true }

// Stream implements api.WSHandler.
func (h *TailHandler) Stream(ctx context.Context, conn *api.WSConn) error {
	route, err := h.D.Routes.TailRoute(ctx, chi.URLParamFromCtx(ctx, string(routeshttp.ParamID)))
	if err != nil {
		return err
	}
	if !route.Tailable() {
		return &api.WSCloseError{Code: int(CloseRouteNotRunning), Reason: string(ReasonNotRunning)}
	}
	masked := route.SourceConnectionID == "" || h.D.Visibility.Masked(ctx, route.SourceConnectionID)
	if err := h.D.Audit.Opened(ctx, conn.Principal(), route, masked); err != nil {
		return err
	}
	session := &livetail.TailSession{RouteID: route.ID, Store: h.D.Store, Masker: h.D.Masker, Masked: masked,
		Limits: h.D.Limits, Log: h.D.Log}
	err = session.Run(ctx, conn)
	if errors.Is(err, livetail.ErrTailUnavailable) {
		return &api.WSCloseError{Code: int(CloseUnavailable), Reason: string(ReasonUnavailable)}
	}
	return err
}

// NewModule mounts the tail endpoint through the platform WebSocket upgrader.
func NewModule(ws *api.WSUpgrader, d Deps) api.Module {
	return ws.Modules(&TailHandler{D: d})
}
