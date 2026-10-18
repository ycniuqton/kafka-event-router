// Package routeshttp serves /api/v1/routes: class-based handlers that decode requests, call the
// routes Managers and render RFC 7807 problems (validation issues as the `issues` member).
package routeshttp

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/bulk"
	"github.com/eventrouter/router/internal/routes/portability"
)

// Path names the routes endpoints.
type Path string

const (
	PathCollection  Path = "/api/v1/routes"
	PathItem        Path = "/api/v1/routes/{id}"
	PathDuplicate   Path = "/api/v1/routes/{id}/duplicate"
	PathValidate    Path = "/api/v1/routes/{id}/validate"
	PathDeploy      Path = "/api/v1/routes/{id}/deploy"
	PathPause       Path = "/api/v1/routes/{id}/pause"
	PathResume      Path = "/api/v1/routes/{id}/resume"
	PathRollback    Path = "/api/v1/routes/{id}/rollback"
	PathDeployments Path = "/api/v1/routes/{id}/deployments"
	PathVersions    Path = "/api/v1/routes/{id}/versions"
	PathVersion     Path = "/api/v1/routes/{id}/versions/{no}"
	PathDiff        Path = "/api/v1/routes/{id}/versions/{a}/diff/{b}"
	PathBulk        Path = "/api/v1/routes/bulk"
	PathUnarchive   Path = "/api/v1/routes/{id}/unarchive"
)

// PathParam names URL parameters.
type PathParam string

const (
	ParamID PathParam = "id"
	ParamNo PathParam = "no"
	ParamA  PathParam = "a"
	ParamB  PathParam = "b"
)

// Permissions of the routes endpoints (keys of access-control's permission catalog).
var (
	PermRead   = auth.Permission{Key: "route.read", Access: auth.AccessRead}
	PermWrite  = auth.Permission{Key: "route.write", Access: auth.AccessWrite}
	PermDeploy = auth.Permission{Key: "route.deploy_nonprod", Access: auth.AccessWrite}
)

// Deps are the managers and conventions the handlers use.
type Deps struct {
	Routes      *routes.RouteManager
	State       *routes.StateManager
	Deploy      *routes.DeployManager
	Versions    *routes.VersionManager
	Validate    *routes.ValidateManager
	Bulk        *bulk.BulkActionManager
	Portability *portability.PortabilityManager
	Replays     *routes.ReplayManager
	Responder   *api.Responder
	Pages       *api.PageParser
}

// NewModule returns every routes handler wrapped by the issues problem writer.
func NewModule(d Deps) api.Module {
	w := IssuesProblemWriter{Responder: d.Responder}
	return api.Modules{
		w.Wrap(&ListHandler{d: d}), w.Wrap(&CreateHandler{d: d}), w.Wrap(&GetHandler{d: d}),
		w.Wrap(&PatchHandler{d: d}), w.Wrap(&ArchiveHandler{d: d}), w.Wrap(&DuplicateHandler{d: d}),
		w.Wrap(&ValidateHandler{d: d}), w.Wrap(&DeployHandler{d: d}), w.Wrap(&PauseHandler{d: d}),
		w.Wrap(&ResumeHandler{d: d}), w.Wrap(&RollbackHandler{d: d}), w.Wrap(&DeploymentsHandler{d: d}),
		w.Wrap(&VersionsHandler{d: d}), w.Wrap(&VersionHandler{d: d}), w.Wrap(&DiffHandler{d: d}),
		w.Wrap(&BulkHandler{d: d}), w.Wrap(&UnarchiveHandler{d: d}), w.Wrap(&ExportHandler{d: d}),
		w.Wrap(&BulkExportHandler{d: d}), w.Wrap(&ImportHandler{d: d}),
		w.Wrap(&ReplayHandler{d: d}), w.Wrap(&ReplayStatusHandler{d: d}),
	}
}

// RouteObject resolves the authorization object of /routes/{id} requests.
func RouteObject(r *http.Request) auth.ObjectRef {
	return auth.ObjectRef{Type: string(audit.ObjectRoute), ID: chi.URLParam(r, string(ParamID))}
}

// Params reads typed path parameters.
type Params struct{}

// RouteID returns the {id} parameter; malformed ids are unknown routes (404).
func (Params) RouteID(r *http.Request) (routes.RouteID, error) {
	raw := chi.URLParam(r, string(ParamID))
	if _, err := uuid.Parse(raw); err != nil {
		return "", routes.ErrRouteNotFound
	}
	return routes.RouteID(raw), nil
}

// VersionNo returns the {no} parameter; non-numbers are unknown versions (404).
func (Params) VersionNo(r *http.Request) (int, error) {
	no, err := strconv.Atoi(chi.URLParam(r, string(ParamNo)))
	if err != nil || no < 1 {
		return 0, routes.ErrVersionNotFound
	}
	return no, nil
}

// Page parses ?limit&cursor into a routes page request.
func (Params) Page(d Deps, r *http.Request) (routes.PageRequest, error) {
	p, err := d.Pages.Parse(r)
	if err != nil {
		return routes.PageRequest{}, err
	}
	return routes.PageRequest{Limit: p.Limit, After: p.After}, nil
}
