// Package http serves the approvals API: list, review detail, approve, reject, withdraw and comments.
// Handlers are class-based api.Handler values; permissions are declared on each RouteSpec.
package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/approvals"
	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// Path names the approvals routes.
type Path string

const (
	PathApprovals Path = "/api/v1/approvals"
	PathApproval  Path = "/api/v1/approvals/{id}"
	PathApprove   Path = "/api/v1/approvals/{id}/approve"
	PathReject    Path = "/api/v1/approvals/{id}/reject"
	PathWithdraw  Path = "/api/v1/approvals/{id}/withdraw"
	PathComments  Path = "/api/v1/approvals/{id}/comments"
)

// Param names path and query parameters.
type Param string

const (
	ParamID      Param = "id"
	ParamStatus  Param = "status"
	ParamTeamID  Param = "teamId"
	ParamRouteID Param = "routeId"
	ParamMine    Param = "mine"
)

// Deps groups what the handlers need.
type Deps struct {
	Approvals approvals.Manager
	Owner     rbac.TeamOwner
	Responder *api.Responder
	Pages     *api.PageParser
}

// NewModule returns the approvals handlers.
func NewModule(d Deps) api.Module {
	return api.Modules{
		&ListHandler{d: d}, &GetHandler{d: d}, &ApproveHandler{d: d}, &RejectHandler{d: d},
		&WithdrawHandler{d: d}, &CommentHandler{d: d},
	}
}

// ErrUnknownApproval is returned for malformed approval ids (the object cannot exist).
var ErrUnknownApproval = problem.NewError(problem.KindNotFound, "This approval request does not exist. It may have been deleted.")

// IDReader parses the approval id path parameter.
type IDReader struct{}

// Read returns the id (404 when malformed).
func (IDReader) Read(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return uuid.Nil, ErrUnknownApproval
	}
	return id, nil
}

// object resolves the approval's team for the Authorizer stage.
func (d Deps) object() auth.ObjectResolver {
	return rbac.ObjectResolvers{}.Owned(rbac.ResourceDeployment, string(ParamID), d.Owner)
}

// FilterReader parses the list filter from the query string (422 on malformed values).
type FilterReader struct{}

// Read returns the filter.
func (FilterReader) Read(r *http.Request) (approvals.ListFilter, error) {
	q := r.URL.Query()
	var f approvals.ListFilter
	verr := api.NewValidationError()
	if raw := q.Get(string(ParamStatus)); raw != "" {
		s := approvals.ApprovalStatus(raw)
		if !(approvals.Statuses{}).Valid(s) {
			verr.Add("/"+string(ParamStatus), "enum", "Use one of pending_approval, rolling_out, active, failed, rejected, cancelled, superseded.")
		}
		f.Status = &s
	}
	f.TeamID = FilterReader{}.uuid(q.Get(string(ParamTeamID)), ParamTeamID, verr)
	f.RouteID = FilterReader{}.uuid(q.Get(string(ParamRouteID)), ParamRouteID, verr)
	if raw := q.Get(string(ParamMine)); raw != "" {
		mine, err := strconv.ParseBool(raw)
		if err != nil {
			verr.Add("/"+string(ParamMine), "boolean", "Use true or false.")
		}
		f.Mine = mine
	}
	if err := verr.OrNil(); err != nil {
		return approvals.ListFilter{}, err
	}
	return f, nil
}

func (FilterReader) uuid(raw string, name Param, verr *api.ValidationError) *uuid.UUID {
	if raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		verr.Add("/"+string(name), "uuid", "Use the id of an existing object.")
		return nil
	}
	return &id
}

// ListHandler serves GET /approvals?status=&teamId=&routeId=&mine=&limit=&cursor=.
type ListHandler struct{ d Deps }

func (h *ListHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathApprovals), Permission: rbac.PermRouteRead.Auth()}
}

func (h *ListHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	page, err := h.d.Pages.Parse(r)
	if err != nil {
		return err
	}
	f, err := FilterReader{}.Read(r)
	if err != nil {
		return err
	}
	items, next, err := h.d.Approvals.List(r.Context(), f, approvals.Page{Limit: page.Limit, After: page.After})
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, api.PageResponse[approvals.Approval]{Items: items, NextCursor: h.d.Pages.Next(next)})
}

// GetHandler serves GET /approvals/{id} (review page) with the request's ETag.
type GetHandler struct{ d Deps }

func (h *GetHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathApproval), Permission: rbac.PermRouteRead.Auth(), Object: h.d.object()}
}

func (h *GetHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := IDReader{}.Read(r)
	if err != nil {
		return err
	}
	detail, err := h.d.Approvals.Get(r.Context(), id)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, detail.Approval.RowVersion, detail)
}

// DecisionRequest is the body of approve / reject.
type DecisionRequest struct {
	Note string `json:"note"`
}

// ApproveHandler serves POST /approvals/{id}/approve (If-Match required; 403 self_approval; 409 already_decided).
type ApproveHandler struct{ d Deps }

func (h *ApproveHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathApprove), Permission: rbac.PermDeploymentApprove.Auth(),
		Object: h.d.object(), Mutating: true, AuditAction: audit.ActionDeploymentApproved, Versioned: true}
}

func (h *ApproveHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, version, req, err := DecisionInput{}.Read(w, r)
	if err != nil {
		return err
	}
	a, err := h.d.Approvals.Approve(r.Context(), id, req.Note, version)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, a.RowVersion, a)
}

// RejectHandler serves POST /approvals/{id}/reject (note required, If-Match required).
type RejectHandler struct{ d Deps }

func (h *RejectHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathReject), Permission: rbac.PermDeploymentApprove.Auth(),
		Object: h.d.object(), Mutating: true, AuditAction: audit.ActionDeploymentRejected, Versioned: true}
}

func (h *RejectHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, version, req, err := DecisionInput{}.Read(w, r)
	if err != nil {
		return err
	}
	a, err := h.d.Approvals.Reject(r.Context(), id, req.Note, version)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, a.RowVersion, a)
}

// WithdrawHandler serves POST /approvals/{id}/withdraw (requester or workspace admin).
type WithdrawHandler struct{ d Deps }

func (h *WithdrawHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathWithdraw), Permission: rbac.PermRouteRead.Auth(),
		Object: h.d.object(), Mutating: true, AuditAction: audit.ActionDeploymentWithdrawn}
}

func (h *WithdrawHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := IDReader{}.Read(r)
	if err != nil {
		return err
	}
	a, err := h.d.Approvals.Withdraw(r.Context(), id)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, a.RowVersion, a)
}

// CommentRequest is the body of POST /approvals/{id}/comments.
type CommentRequest struct {
	Body string `json:"body"`
}

// CommentHandler serves POST /approvals/{id}/comments (anyone who may read the route).
type CommentHandler struct{ d Deps }

func (h *CommentHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathComments), Permission: rbac.PermRouteRead.Auth(),
		Object: h.d.object(), Mutating: true, AuditAction: audit.ActionApprovalCommented}
}

func (h *CommentHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := IDReader{}.Read(r)
	if err != nil {
		return err
	}
	req, err := api.Decoder[CommentRequest]{}.Decode(w, r)
	if err != nil {
		return err
	}
	c, err := h.d.Approvals.Comment(r.Context(), id, req.Body)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusCreated, c)
}

// DecisionInput reads the id, the required If-Match version and the optional note of a decision.
type DecisionInput struct{}

// Read parses the request (404 on a malformed id, 428 without If-Match).
func (DecisionInput) Read(w http.ResponseWriter, r *http.Request) (uuid.UUID, store.RowVersion, DecisionRequest, error) {
	id, err := IDReader{}.Read(r)
	if err != nil {
		return uuid.Nil, 0, DecisionRequest{}, err
	}
	version, err := api.Precondition{}.Require(r)
	if err != nil {
		return uuid.Nil, 0, DecisionRequest{}, err
	}
	req, err := api.Decoder[DecisionRequest]{AllowEmpty: true}.Decode(w, r)
	return id, version, req, err
}
