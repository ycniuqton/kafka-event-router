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

// JobBody is the body of POST /routes/{id}/dlq/jobs: a match (default: everything) or explicit ids,
// the replay rate and a note (design 00001 §4.4).
type JobBody struct {
	Action dlq.ActionKind `json:"action"`
	Match  *MatchBody     `json:"match"`
	IDs    []string       `json:"ids"`
	Rate   *int           `json:"ratePerSecond"`
	Note   string         `json:"note"`
}

// JobRequestOf validates job bodies.
type JobRequestOf struct{}

// Request returns the create request of b.
func (JobRequestOf) Request(b JobBody) (jobs.CreateRequest, error) {
	req := jobs.CreateRequest{IDs: b.IDs, Rate: int(jobs.DefaultRate), Note: b.Note}
	if len(b.IDs) > 0 && b.Match != nil {
		return jobs.CreateRequest{}, dlq.ErrSelectionShape.At(string(dlq.PathRoot))
	}
	if len(b.IDs) > int(jobs.MaxJobIDs) {
		return jobs.CreateRequest{}, dlq.ErrSelectionTooBig.At(string(dlq.PathIDs))
	}
	if _, err := (dlq.RefCodec{}).DecodeAll(b.IDs); err != nil {
		return jobs.CreateRequest{}, err
	}
	if b.Rate != nil && !jobs.Rate(*b.Rate).Valid() {
		return jobs.CreateRequest{}, dlq.ErrInvalidRate.At(string(dlq.PathRate))
	}
	if b.Rate != nil {
		req.Rate = *b.Rate
	}
	if len([]rune(b.Note)) > int(dlq.MaxNoteLength) {
		return jobs.CreateRequest{}, dlq.ErrInvalidNote.At(string(dlq.PathNote))
	}
	if b.Match != nil {
		q, err := RequestValidator{}.Match(*b.Match)
		req.Match = q
		return req, err
	}
	return req, nil
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
	req, err := JobRequestOf{}.Request(body)
	if err != nil {
		return err
	}
	j, err := svc.CreateWith(r.Context(), body.Action, Caller{}.Actor(r), chi.URLParam(r, string(ParamRouteID)), req)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusAccepted, CallerJobs{}.One(r, j))
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
	return h.d.Responder.JSON(w, http.StatusOK, JobList{Items: CallerJobs{}.All(r, list)})
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
	return h.d.Responder.JSON(w, http.StatusOK, CallerJobs{}.One(r, j))
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
	return h.d.Responder.JSON(w, http.StatusOK, CallerJobs{}.One(r, j))
}
