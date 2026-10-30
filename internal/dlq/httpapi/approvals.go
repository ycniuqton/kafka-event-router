package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/jobs"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/rbac"
)

// PermApprove is the permission to decide a replay held back by the 4-eyes gate (same as deploy approvals).
var PermApprove = rbac.PermDeploymentApprove.Auth()

// DecisionBody is the body of approve / reject.
type DecisionBody struct {
	Note string `json:"note"`
}

// DecisionCall decides a pending replay job as the caller.
type DecisionCall struct{ d Deps }

// Run applies approve to the job of the request.
func (c DecisionCall) Run(w http.ResponseWriter, r *http.Request, approve bool) error {
	svc, err := JobService(c).Get()
	if err != nil {
		return err
	}
	body, err := api.Decoder[DecisionBody]{AllowEmpty: true}.Decode(w, r)
	if err != nil {
		return err
	}
	if len([]rune(body.Note)) > int(dlq.MaxNoteLength) {
		return dlq.ErrInvalidNote.At(string(dlq.PathNote))
	}
	d := jobs.Decision{Approve: approve, By: Caller{}.Actor(r).UserID, Note: body.Note}
	j, err := svc.Decide(r.Context(), chi.URLParam(r, string(ParamJobID)), d)
	if err != nil {
		return err
	}
	return c.d.Responder.JSON(w, http.StatusOK, CallerJobs{}.One(r, j))
}

// ApproveJobHandler serves POST /dlq/jobs/{jobId}/approve.
type ApproveJobHandler struct{ d Deps }

func (h *ApproveJobHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathJobApprove), Permission: PermApprove, Object: JobObject(h.d),
		Mutating: true, AuditAction: dlq.ActionDLQReplayApproved}
}

func (h *ApproveJobHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	return DecisionCall(*h).Run(w, r, true)
}

// RejectJobHandler serves POST /dlq/jobs/{jobId}/reject.
type RejectJobHandler struct{ d Deps }

func (h *RejectJobHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathJobReject), Permission: PermApprove, Object: JobObject(h.d),
		Mutating: true, AuditAction: dlq.ActionDLQReplayRejected}
}

func (h *RejectJobHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	return DecisionCall(*h).Run(w, r, false)
}

// PendingJobsHandler serves GET /dlq/jobs?status=pending_approval: replays waiting for a second person on
// routes the caller may read (the approvals inbox shows which ones the caller may decide).
type PendingJobsHandler struct{ d Deps }

func (h *PendingJobsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathJobsList)}
}

func (h *PendingJobsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	svc, err := JobService(*h).Get()
	if err != nil {
		return err
	}
	if st := r.URL.Query().Get(string(QueryStatus)); st != "" && jobs.Status(st) != jobs.StatusPendingApproval {
		return dlq.ErrInvalidFilter.At(string(dlq.PathQuery) + "/" + string(QueryStatus))
	}
	pending, err := svc.Pending(r.Context())
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, JobList{Items: CallerJobs{}.All(r, PendingFilter{d: h.d}.Readable(r, pending))})
}

// PendingFilter keeps the jobs of routes the caller may read.
type PendingFilter struct{ d Deps }

// Readable returns the readable jobs of list.
func (f PendingFilter) Readable(r *http.Request, list []jobs.Job) []jobs.Job {
	p := auth.PrincipalFrom(r.Context())
	out := []jobs.Job{}
	for _, j := range list {
		route, err := f.d.Manager.Route(r.Context(), j.RouteID)
		if err != nil {
			continue
		}
		obj := auth.ObjectRef{Type: string(audit.ObjectRoute), ID: route.ID, TeamID: route.TeamID}
		if f.d.Authorizer.Authorize(r.Context(), p, PermRead, obj) == nil {
			out = append(out, j)
		}
	}
	return out
}
