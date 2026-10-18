// Package httpapi serves the caller's API tokens: GET/POST /api/v1/auth/tokens and
// DELETE /api/v1/auth/tokens/{id}. Only interactive sessions may manage tokens.
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/auth/apitoken"
	"github.com/eventrouter/router/internal/platform/api"
)

// Path names the token routes.
type Path string

const (
	PathTokens Path = "/api/v1/auth/tokens"
	PathToken  Path = "/api/v1/auth/tokens/{id}"
)

// Param names URL parameters.
type Param string

const ParamID Param = "id"

// Deps groups what the handlers need.
type Deps struct {
	Tokens    *apitoken.Manager
	Responder *api.Responder
}

// NewModule returns the API-token module.
func NewModule(d Deps) api.Module {
	return api.Modules{&ListHandler{d: d}, &CreateHandler{d: d}, &RevokeHandler{d: d}}
}

// ListResponse is the body of GET /api/v1/auth/tokens.
type ListResponse struct {
	Items      []apitoken.View   `json:"items"`
	NextCursor *string           `json:"next_cursor"`
	Scopes     []auth.TokenScope `json:"scopes"`
}

// ListHandler serves GET /api/v1/auth/tokens.
type ListHandler struct{ d Deps }

func (h *ListHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathTokens)}
}

func (h *ListHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	items, err := h.d.Tokens.List(r.Context(), auth.PrincipalFrom(r.Context()))
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, ListResponse{Items: items, Scopes: h.d.Tokens.Scopes()})
}

// CreateHandler serves POST /api/v1/auth/tokens (201, secret shown once).
type CreateHandler struct{ d Deps }

func (h *CreateHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathTokens), Mutating: true, AuditAction: apitoken.ActionCreated}
}

func (h *CreateHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	req, err := api.Decoder[apitoken.CreateRequest]{}.Decode(w, r)
	if err != nil {
		return err
	}
	created, err := h.d.Tokens.Create(r.Context(), auth.PrincipalFrom(r.Context()), req)
	if err != nil {
		return err
	}
	w.Header().Set(string(api.HeaderLocation), string(PathTokens)+"/"+created.ID.String())
	return h.d.Responder.JSON(w, http.StatusCreated, created)
}

// RevokeHandler serves DELETE /api/v1/auth/tokens/{id} (204).
type RevokeHandler struct{ d Deps }

func (h *RevokeHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodDelete, Pattern: string(PathToken), Mutating: true, AuditAction: apitoken.ActionRevoked}
}

func (h *RevokeHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := uuid.Parse(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return apitoken.ErrNotFound
	}
	if err := h.d.Tokens.Revoke(r.Context(), auth.PrincipalFrom(r.Context()), id); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}
