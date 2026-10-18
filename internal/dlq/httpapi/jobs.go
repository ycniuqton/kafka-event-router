package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/jobs"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac"
)

// ErrJobsDisabled answers job endpoints when the API runs without the job queue.
var ErrJobsDisabled = problem.NewError(problem.KindUnavailable, "Background dead-letter jobs are not available on this server.")

// JobBody is the body of POST /routes/{id}/dlq/jobs.
type JobBody struct {
	Action dlq.ActionKind `json:"action"`
	Match  MatchBody      `json:"match"`
}

// JobService returns the configured job service or the disabled problem.
type JobService struct{ d Deps }

// Get returns the service.
func (s JobService) Get() (*jobs.Service, error) {
	if s.d.Jobs == nil {
		return nil, ErrJobsDisabled
	}
	return s.d.Jobs, nil
}

// JobTeams resolves the owning team of a job (its route's team) for authorization.
type JobTeams struct{ d Deps }

// TeamOf implements rbac.TeamOwner.
func (t JobTeams) TeamOf(ctx context.Context, id string) (string, error) {
	svc, err := JobService(t).Get()
	if err != nil {
		return "", err
	}
	j, err := svc.Get(ctx, id)
	if err != nil {
		return "", err
	}
	route, err := t.d.Manager.Route(ctx, j.RouteID)
	return route.TeamID, err
}

// JobObject resolves the authorization object of /dlq/jobs/{jobId} requests.
func JobObject(d Deps) auth.ObjectResolver {
	return rbac.ObjectResolvers{}.Owned(rbac.ResourceRoute, string(ParamJobID), JobTeams{d: d})
}

// CreateJobHandler serves POST /routes/{id}/dlq/jobs → 202 {jobId}.
type CreateJobHandler struct{ d Deps }

func (h *CreateJobHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathJobs), Permission: PermAct, Object: RouteObject,
		Mutating: true, AuditAction: dlq.ActionDLQJobCreated}
}

func (h *CreateJobHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	svc, err := JobService(*h).Get()
	if err != nil {
		return err
	}
	body, err := api.Decoder[JobBody]{}.Decode(w, r)
	if err != nil {
		return err
	}
	q, err := RequestValidator{}.Match(body.Match)
	if err != nil {
		return err
	}
	j, err := svc.Create(r.Context(), body.Action, Caller{}.Actor(r), chi.URLParam(r, string(ParamRouteID)), q)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusAccepted, j)
}

// RouteJobsHandler serves GET /routes/{id}/dlq/jobs (newest jobs of the route).
type RouteJobsHandler struct{ d Deps }

func (h *RouteJobsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathJobs), Permission: PermRead, Object: RouteObject}
}

func (h *RouteJobsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	svc, err := JobService(*h).Get()
	if err != nil {
		return err
	}
	list, err := svc.Recent(r.Context(), chi.URLParam(r, string(ParamRouteID)))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, JobList{Items: list})
}

// JobList is the body of the route job listing.
type JobList struct {
	Items []jobs.Job `json:"items"`
}

// JobStatusHandler serves GET /dlq/jobs/{jobId}.
type JobStatusHandler struct{ d Deps }

func (h *JobStatusHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathJob), Permission: PermRead, Object: JobObject(h.d)}
}

func (h *JobStatusHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	svc, err := JobService(*h).Get()
	if err != nil {
		return err
	}
	j, err := svc.Get(r.Context(), chi.URLParam(r, string(ParamJobID)))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, j)
}

// CancelJobHandler serves POST /dlq/jobs/{jobId}/cancel.
type CancelJobHandler struct{ d Deps }

func (h *CancelJobHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathJobCancel), Permission: PermAct, Object: JobObject(h.d),
		Mutating: true, AuditAction: dlq.ActionDLQJobCancelled}
}

func (h *CancelJobHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	svc, err := JobService(*h).Get()
	if err != nil {
		return err
	}
	j, err := svc.Cancel(r.Context(), chi.URLParam(r, string(ParamJobID)))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, j)
}
