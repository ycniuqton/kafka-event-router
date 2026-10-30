// Package httpapi serves the dead-letter endpoints under /api/v1: class-based handlers that parse
// the request, call dlq.Manager and render DTOs; domain errors are classified problems.
package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/jobs"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/rbac"
)

// Path names the dead-letter endpoints.
type Path string

const (
	PathRouteList    Path = "/api/v1/routes/{id}/dlq"
	PathRouteSummary Path = "/api/v1/routes/{id}/dlq/summary"
	PathRouteItem    Path = "/api/v1/routes/{id}/dlq/{dlqId}"
	PathRetry        Path = "/api/v1/routes/{id}/dlq/retry"
	PathDiscard      Path = "/api/v1/routes/{id}/dlq/discard"
	PathRestore      Path = "/api/v1/routes/{id}/dlq/restore"
	PathExport       Path = "/api/v1/routes/{id}/dlq/export"
	PathStudio       Path = "/api/v1/routes/{id}/dlq/{dlqId}/studio"
	PathGlobalList   Path = "/api/v1/dlq"
	PathGlobalSum    Path = "/api/v1/dlq/summary"
	PathJobs         Path = "/api/v1/routes/{id}/dlq/jobs"
	PathJob          Path = "/api/v1/dlq/jobs/{jobId}"
	PathJobCancel    Path = "/api/v1/dlq/jobs/{jobId}/cancel"
)

// PathParam names URL parameters.
type PathParam string

const (
	ParamRouteID PathParam = "id"
	ParamDLQID   PathParam = "dlqId"
	ParamJobID   PathParam = "jobId"
)

// Permissions of the endpoints (keys of access-control's catalog).
var (
	PermRead   = rbac.PermDLQRead.Auth()
	PermAct    = rbac.PermDLQAct.Auth()
	PermStudio = rbac.PermRouteWrite.Auth()
)

// Deps are what the handlers use.
type Deps struct {
	Manager    *dlq.Manager
	Jobs       *jobs.Service
	Authorizer auth.Authorizer
	Recorder   audit.Recorder
	Responder  *api.Responder
}

// NewModule returns every handler (MVP + P2 restore and bulk jobs + 00001 workflow).
func NewModule(d Deps) api.Module {
	return api.Modules{
		&ListHandler{d: d}, &SummaryHandler{d: d}, &DetailHandler{d: d}, &RetryHandler{d: d}, &DiscardHandler{d: d}, &RestoreHandler{d: d},
		&ExportHandler{d: d}, &StudioHandler{d: d}, &GlobalListHandler{d: d}, &GlobalSummaryHandler{d: d},
		&CreateJobHandler{d: d}, &RouteJobsHandler{d: d}, &JobStatusHandler{d: d}, &CancelJobHandler{d: d},
		&GroupsHandler{d: d}, &GlobalGroupsHandler{d: d}, &ReplayHandler{d: d}, &DryRunHandler{d: d},
		&ApproveJobHandler{d: d}, &RejectJobHandler{d: d}, &PendingJobsHandler{d: d},
	}
}

// RouteObject resolves the authorization object of /routes/{id}/dlq requests.
func RouteObject(r *http.Request) auth.ObjectRef {
	return auth.ObjectRef{Type: string(audit.ObjectRoute), ID: chi.URLParam(r, string(ParamRouteID))}
}

// Caller reads who makes the request.
type Caller struct{}

// Viewer returns the request principal as a viewer.
func (Caller) Viewer(r *http.Request) dlq.Viewer {
	return dlq.Viewer{UserID: auth.PrincipalFrom(r.Context()).UserID.String()}
}

// Actor returns the request principal as an actor.
func (Caller) Actor(r *http.Request) dlq.Actor {
	return dlq.Actor{UserID: auth.PrincipalFrom(r.Context()).UserID.String()}
}

// CallerJobs marks jobs requested by the caller.
type CallerJobs struct{}

// One returns j marked for the request's principal.
func (CallerJobs) One(r *http.Request, j jobs.Job) jobs.Job {
	return j.ForCaller(Caller{}.Actor(r).UserID)
}

// All returns list marked for the request's principal.
func (c CallerJobs) All(r *http.Request, list []jobs.Job) []jobs.Job {
	out := make([]jobs.Job, 0, len(list))
	for _, j := range list {
		out = append(out, c.One(r, j))
	}
	return out
}

// ReadableRoutes keeps the routes whose dead letters the caller may read.
type ReadableRoutes struct {
	Authorizer auth.Authorizer
}

// Filter returns routes allowed by dlq.read (and in only, when given).
func (f ReadableRoutes) Filter(ctx context.Context, routes []dlq.RouteInfo, only map[string]bool) []dlq.RouteInfo {
	p := auth.PrincipalFrom(ctx)
	out := []dlq.RouteInfo{}
	for _, r := range routes {
		if len(only) > 0 && !only[r.ID] {
			continue
		}
		obj := auth.ObjectRef{Type: string(audit.ObjectRoute), ID: r.ID, TeamID: r.TeamID}
		if f.Authorizer.Authorize(ctx, p, PermRead, obj) == nil {
			out = append(out, r)
		}
	}
	return out
}
