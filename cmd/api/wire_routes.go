package main

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/approvals"
	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/assembly"
)

// routesBuilds memoises the routes assembly per Wiring so in-process consumers (route-runtime,
// monitoring, access-control approvals) share the module's managers.
var routesBuilds sync.Map

// routesBuild is one memoised build result.
type routesBuild struct {
	once       sync.Once
	components *assembly.Components
	err        error
}

// RoutesComponents builds (once) and returns the routes managers; Ports may be extended before first use.
func (w *Wiring) RoutesComponents() (*assembly.Components, error) {
	v, _ := routesBuilds.LoadOrStore(w, &routesBuild{})
	b := v.(*routesBuild)
	b.once.Do(func() { b.components, b.err = w.buildRoutes(context.Background()) })
	return b.components, b.err
}

// buildRoutes composes the routes feature from the shared wiring.
func (w *Wiring) buildRoutes(ctx context.Context) (*assembly.Components, error) {
	settings, err := assembly.NewSettingsLoader().Load()
	if err != nil {
		return nil, err
	}
	w.registerRoutesAuditActions()
	return assembly.Build(ctx, assembly.Deps{DB: w.Deps.DB, Tx: w.Tx, Notifier: w.Notifier, Recorder: w.Deps.Audit,
		Responder: w.Responder, Pages: w.Pages, Log: w.Deps.Log, Settings: settings, Ports: w.routesPorts()})
}

// registerRoutesAuditActions adds the routes actions missing from access-control's base catalog.
func (w *Wiring) registerRoutesAuditActions() {
	if w.Access == nil || w.Access.Catalog == nil {
		return
	}
	for _, a := range routes.ExtraAuditActions {
		w.Access.Catalog.Register(a, audit.ActionMeta{Category: audit.CategoryChange})
	}
}

// routesModule returns the /api/v1/routes module; a build error disables it (logged) instead of failing the API.
func (s *apiService) routesModule() api.Module {
	c, err := s.wiring.RoutesComponents()
	if err != nil {
		s.wiring.Deps.Log.Error("routes module disabled", string(logging.AttrError), err)
		return api.Modules{}
	}
	replayProgressFeature.start(s.wiring)
	return c.Module
}

// RoutesApprovalRequests registers routes' pending deployments with access-control approvals (notifications,
// cancelling older pending requests) once the deploy transaction committed.
type RoutesApprovalRequests struct {
	Approvals approvals.Manager
}

// RequestApproval implements routes.ApprovalRequester.
func (r RoutesApprovalRequests) RequestApproval(ctx context.Context, d routes.Deployment) error {
	dep, err := uuid.Parse(string(d.ID))
	if err != nil {
		return err
	}
	route, err := uuid.Parse(string(d.RouteID))
	if err != nil {
		return err
	}
	_, err = r.Approvals.Request(ctx, approvals.RequestInput{DeploymentID: dep, RouteID: route})
	return err
}
