package controlapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
)

// PublicPath names the public route-runtime endpoints.
type PublicPath string

const (
	PathRouteRuntime PublicPath = "/api/v1/routes/{id}/runtime"
	PathWorkers      PublicPath = "/api/v1/workers"
)

// ErrRouteNotFound is the 404 of an unknown route.
var ErrRouteNotFound = problem.NewError(problem.KindNotFound, "This route does not exist. It may have been archived or deleted.")

// RuntimeView is the body of GET /routes/{id}/runtime.
type RuntimeView struct {
	ObservedState    domain.ObservedState `json:"observedState"`
	RunningVersionNo *int                 `json:"runningVersionNo"`
	HaltedAt         *time.Time           `json:"haltedAt,omitempty"`
	HaltReason       domain.HaltReason    `json:"haltReason,omitempty"`
	ErrorMessage     string               `json:"errorMessage,omitempty"`
	Lag              int64                `json:"lag"`
	Replicas         []ReplicaItem        `json:"replicas"`
}

// ReplicaItem is one replica of RuntimeView.
type ReplicaItem struct {
	WorkerID   string             `json:"workerId"`
	State      domain.RunnerState `json:"state"`
	VersionNo  *int               `json:"versionNo"`
	Lag        int64              `json:"lag"`
	ReportedAt *time.Time         `json:"reportedAt"`
	Error      string             `json:"errorMessage,omitempty"`
}

// WorkerItem is one row of GET /workers.
type WorkerItem struct {
	state.Worker
	Alive bool `json:"alive"`
}

// PublicDeps are the collaborators of the public handlers.
type PublicDeps struct {
	Stores    state.Stores
	Responder *api.Responder
	Pages     *api.PageParser
	Timings   domain.Timings
	Now       func() time.Time
}

// NewPublicModule returns GET /api/v1/routes/{id}/runtime and GET /api/v1/workers.
func NewPublicModule(d PublicDeps) api.Module {
	return api.Modules{&RouteRuntimeHandler{d: d, views: RuntimeViews{Stores: d.Stores}}, &WorkersHandler{d: d}}
}

// RouteObject resolves the authorization object of /routes/{id}/runtime.
func RouteObject(r *http.Request) auth.ObjectRef {
	return auth.ObjectRef{Type: string(audit.ObjectRoute), ID: chi.URLParam(r, string(domain.PathParam))}
}

// RouteRuntimeHandler serves the aggregated runtime of one route.
type RouteRuntimeHandler struct {
	d     PublicDeps
	views RuntimeViews
}

func (h *RouteRuntimeHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathRouteRuntime), Permission: rbac.PermRouteRead.Auth(), Object: RouteObject}
}

func (h *RouteRuntimeHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	view, err := h.views.Of(r.Context(), chi.URLParam(r, string(domain.PathParam)))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, view)
}

// WorkersHandler serves the cursor-paged worker list (workspace admins).
type WorkersHandler struct{ d PublicDeps }

func (h *WorkersHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathWorkers), Permission: rbac.PermSettingsManage.Auth()}
}

func (h *WorkersHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	page, err := h.d.Pages.Parse(r)
	if err != nil {
		return err
	}
	list, next, err := h.d.Stores.Workers.PageWorkers(r.Context(), page.Limit, page.After)
	if err != nil {
		return err
	}
	now := h.d.Now()
	items := make([]WorkerItem, 0, len(list))
	for _, wk := range list {
		items = append(items, WorkerItem{Worker: wk, Alive: wk.Alive(now, h.d.Timings.LeaseTTL)})
	}
	return h.d.Responder.JSON(w, http.StatusOK, api.PageResponse[WorkerItem]{Items: items, NextCursor: h.d.Pages.Next(next)})
}

// RuntimeViews builds the public runtime view of a route.
type RuntimeViews struct {
	Stores state.Stores
}

// Of returns the view; a route never reported yet is paused with no replicas.
func (v RuntimeViews) Of(ctx context.Context, routeID string) (RuntimeView, error) {
	ok, err := v.Stores.Routes.Exists(ctx, routeID)
	if err != nil || !ok {
		return RuntimeView{}, RouteErr{}.Or(err)
	}
	row, err := v.Stores.Runtime.Get(ctx, routeID)
	if err != nil && !(RouteErr{}).Missing(err) {
		return RuntimeView{}, err
	}
	view := RuntimeView{ObservedState: domain.ObservedPaused, Replicas: []ReplicaItem{}}
	if err == nil {
		view = RuntimeView{ObservedState: row.Observed, HaltedAt: row.HaltedAt, HaltReason: row.HaltReason, ErrorMessage: row.ErrorMessage,
			Lag: row.Lag, Replicas: []ReplicaItem{}, RunningVersionNo: VersionNumber{}.OrNil(row.RunningVersionNo)}
	}
	rows, err := v.Stores.Assignments.ForRoute(ctx, routeID)
	if err != nil {
		return RuntimeView{}, err
	}
	for _, a := range rows {
		view.Replicas = append(view.Replicas, v.replica(ctx, a))
	}
	return view, nil
}

func (v RuntimeViews) replica(ctx context.Context, a state.AssignmentRow) ReplicaItem {
	item := ReplicaItem{WorkerID: a.WorkerID, State: a.RunnerState, Lag: a.Lag, ReportedAt: a.ReportedAt, Error: a.ErrorMessage}
	if a.ReportedVersionID != "" {
		if no, err := v.Stores.Routes.VersionNo(ctx, a.ReportedVersionID); err == nil {
			item.VersionNo = &no
		}
	}
	if item.State == "" {
		item.State = domain.RunnerStarting
	}
	return item
}

// VersionNumber renders optional version numbers.
type VersionNumber struct{}

// OrNil returns nil for unknown (non-positive) numbers.
func (VersionNumber) OrNil(n int) *int {
	if n <= 0 {
		return nil
	}
	return &n
}
