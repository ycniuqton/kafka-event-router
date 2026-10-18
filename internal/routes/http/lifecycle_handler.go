package routeshttp

import (
	"encoding/json"
	"net/http"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/internal/store"
)

// ValidateBody is the optional body of POST /routes/{id}/validate.
type ValidateBody struct {
	Draft json.RawMessage `json:"draft"`
	Mode  string          `json:"mode"`
}

// DeployBody is the optional body of POST /routes/{id}/deploy.
type DeployBody struct {
	ChangeNote string `json:"changeNote"`
}

// RollbackBody is the body of POST /routes/{id}/rollback.
type RollbackBody struct {
	Version    int    `json:"version"`
	ChangeNote string `json:"changeNote"`
}

// ValidateHandler serves POST /routes/{id}/validate (read-only).
type ValidateHandler struct{ d Deps }

func (h *ValidateHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathValidate), Permission: PermRead, Object: RouteObject, BodyLimit: api.BodyLimitLarge}
}

func (h *ValidateHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	body, err := api.Decoder[ValidateBody]{Limit: api.BodyLimitLarge, AllowEmpty: true}.Decode(w, r)
	if err != nil {
		return err
	}
	res, err := h.d.Validate.Validate(r.Context(), id, body.Draft, validation.ValidationMode(body.Mode))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, res)
}

// DeployHandler serves POST /routes/{id}/deploy (If-Match required).
type DeployHandler struct{ d Deps }

func (h *DeployHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathDeploy), Permission: PermDeploy, Object: RouteObject,
		Mutating: true, AuditAction: routes.ActionRouteDeployed, Versioned: true}
}

func (h *DeployHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	expected, err := api.Precondition{}.Require(r)
	if err != nil {
		return err
	}
	body, err := api.Decoder[DeployBody]{AllowEmpty: true}.Decode(w, r)
	if err != nil {
		return err
	}
	res, err := h.d.Deploy.Deploy(r.Context(), id, routes.Revision(expected), body.ChangeNote)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusAccepted, store.RowVersion(res.Route.Revision), res)
}

// RollbackHandler serves POST /routes/{id}/rollback.
type RollbackHandler struct{ d Deps }

func (h *RollbackHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathRollback), Permission: PermDeploy, Object: RouteObject,
		Mutating: true, AuditAction: routes.ActionRouteRolledBack}
}

func (h *RollbackHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	body, err := api.Decoder[RollbackBody]{}.Decode(w, r)
	if err != nil {
		return err
	}
	if body.Version < 1 {
		return api.NewValidationError().Add(string(routes.PathVersion), string(routes.CodeRequired), "Pick the version to roll back to.")
	}
	res, err := h.d.Deploy.Rollback(r.Context(), id, body.Version, body.ChangeNote)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusAccepted, store.RowVersion(res.Route.Revision), res)
}

// PauseHandler serves POST /routes/{id}/pause.
type PauseHandler struct{ d Deps }

func (h *PauseHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathPause), Permission: PermWrite, Object: RouteObject,
		Mutating: true, AuditAction: routes.ActionRoutePaused}
}

func (h *PauseHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	d, err := h.d.State.Pause(r.Context(), id)
	if err != nil {
		return err
	}
	return Writer{d: h.d}.Detail(w, http.StatusOK, d)
}

// ResumeHandler serves POST /routes/{id}/resume.
type ResumeHandler struct{ d Deps }

func (h *ResumeHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathResume), Permission: PermWrite, Object: RouteObject,
		Mutating: true, AuditAction: routes.ActionRouteResumed}
}

func (h *ResumeHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	d, err := h.d.State.Resume(r.Context(), id)
	if err != nil {
		return err
	}
	return Writer{d: h.d}.Detail(w, http.StatusOK, d)
}

// DeploymentsHandler serves GET /routes/{id}/deployments.
type DeploymentsHandler struct{ d Deps }

func (h *DeploymentsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathDeployments), Permission: PermRead, Object: RouteObject}
}

func (h *DeploymentsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := Params{}.RouteID(r)
	if err != nil {
		return err
	}
	page, err := Params{}.Page(h.d, r)
	if err != nil {
		return err
	}
	items, next, err := h.d.Versions.Deployments(r.Context(), id, page)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, api.PageResponse[routes.Deployment]{Items: items, NextCursor: h.d.Pages.Next(next)})
}
