package connhttp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/pkg/kafka"
)

// TeamMessage holds owner-team validation texts.
type TeamMessage string

const (
	MsgTeamNotMember TeamMessage = "You are not a member of this team. Pick one of your teams."
	MsgTeamRequired  TeamMessage = "You are not a member of any team yet. Ask an admin to add you to a team."
	MsgEnvFilter     TeamMessage = "Filter by dev, staging or prod."
	MsgStatusFilter  TeamMessage = "Filter by unknown, healthy, degraded, unreachable or auth_failed."
)

// OwnerTeamPolicy picks the owner team of a new connection from the body or the caller's teams.
type OwnerTeamPolicy struct{}

// Resolve returns the requested team when the caller belongs to it, else the caller's first writable team.
func (OwnerTeamPolicy) Resolve(p auth.Principal, owner *OwnerInput) (uuid.UUID, error) {
	if owner != nil && owner.TeamID != uuid.Nil {
		for _, t := range p.Teams {
			if t.TeamID == owner.TeamID {
				return t.TeamID, nil
			}
		}
		return uuid.Nil, api.NewValidationError().Add(string(connections.PathOwnerTeam), string(kafka.CodeInvalid), string(MsgTeamNotMember))
	}
	for _, t := range p.Teams {
		if t.Role == auth.RoleEditor || t.Role == auth.RoleAdmin {
			return t.TeamID, nil
		}
	}
	if len(p.Teams) > 0 {
		return p.Teams[0].TeamID, nil
	}
	return uuid.Nil, api.NewValidationError().Add(string(connections.PathOwnerTeam), string(kafka.CodeRequired), string(MsgTeamRequired))
}

// ListHandler serves GET /api/v1/connections.
type ListHandler struct{ d Deps }

// Route implements api.Handler.
func (h *ListHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathCollection), Permission: PermRead}
}

// Serve implements api.Handler.
func (h *ListHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	page, err := h.d.Pages.Parse(r)
	if err != nil {
		return err
	}
	q, err := ListQueryParser{}.Parse(r, page)
	if err != nil {
		return err
	}
	result, counts, err := h.d.Service.List(r.Context(), q)
	if err != nil {
		return err
	}
	items := make([]ConnectionDTO, 0, len(result.Items))
	for _, c := range result.Items {
		items = append(items, Presenter{}.Connection(c, UsedByDTO{Count: counts[c.ID]}))
	}
	return h.d.Responder.JSON(w, http.StatusOK, api.PageResponse[ConnectionDTO]{Items: items, NextCursor: h.d.Pages.Next(result.Next)})
}

// ListQueryParser reads the env/status/q filters.
type ListQueryParser struct{}

// Parse validates the filters.
func (ListQueryParser) Parse(r *http.Request, page api.PageRequest) (connections.ListQuery, error) {
	v := r.URL.Query()
	q := connections.ListQuery{
		Env: connections.EnvTag(v.Get(string(QueryEnv))), Status: connections.HealthStatus(v.Get(string(QueryStatus))),
		Text: v.Get(string(QueryText)), Limit: page.Limit, After: page.After,
	}
	verr := api.NewValidationError()
	if q.Env != "" && !q.Env.Valid() {
		verr.Add("/"+string(QueryEnv), string(kafka.CodeInvalid), string(MsgEnvFilter))
	}
	if q.Status != "" && !q.Status.Valid() {
		verr.Add("/"+string(QueryStatus), string(kafka.CodeInvalid), string(MsgStatusFilter))
	}
	return q, verr.OrNil()
}

// GetHandler serves GET /api/v1/connections/{id}.
type GetHandler struct{ d Deps }

// Route implements api.Handler.
func (h *GetHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathItem), Permission: PermRead, Object: h.d.Object}
}

// Serve implements api.Handler.
func (h *GetHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := RouteID{}.Of(r)
	if err != nil {
		return err
	}
	c, routes, err := h.d.Service.Get(r.Context(), id)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, c.Version, Presenter{}.Connection(c, UsedByDTO{Count: len(routes), Routes: routes}))
}

// CreateHandler serves POST /api/v1/connections.
type CreateHandler struct{ d Deps }

// Route implements api.Handler.
func (h *CreateHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathCollection), Permission: PermWrite, Mutating: true, AuditAction: connections.ActionCreated}
}

// Serve implements api.Handler.
func (h *CreateHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	in, err := api.Decoder[ConnectionInput]{}.Decode(w, r)
	if err != nil {
		return err
	}
	team, err := OwnerTeamPolicy{}.Resolve(auth.PrincipalFrom(r.Context()), in.Owner)
	if err != nil {
		return err
	}
	if err := (TeamGuard{authz: h.d.Authz}).Require(r.Context(), PermWrite, team); err != nil {
		return err
	}
	c, err := h.d.Service.Create(r.Context(), in.Draft(team))
	if err != nil {
		return err
	}
	w.Header().Set(string(api.HeaderLocation), string(PathCollection)+"/"+c.ID.String())
	return h.d.Responder.Versioned(w, http.StatusCreated, c.Version, Presenter{}.Connection(c, UsedByDTO{Routes: []connections.RouteRef{}}))
}

// UpdateHandler serves PATCH /api/v1/connections/{id} (JSON merge patch, If-Match required).
type UpdateHandler struct{ d Deps }

// Route implements api.Handler.
func (h *UpdateHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPatch, Pattern: string(PathItem), Permission: PermWrite, Object: h.d.Object, Mutating: true, AuditAction: connections.ActionUpdated, Versioned: true}
}

// Serve implements api.Handler.
func (h *UpdateHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := RouteID{}.Of(r)
	if err != nil {
		return err
	}
	expected, err := api.Precondition{}.Require(r)
	if err != nil {
		return err
	}
	current, routes, err := h.d.Service.Get(r.Context(), id)
	if err != nil {
		return err
	}
	in, err := MergePatcher{}.Apply(w, r, InputOf(current))
	if err != nil {
		return err
	}
	c, err := h.d.Service.Update(r.Context(), id, expected, in.Draft(current.TeamID))
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, c.Version, Presenter{}.Connection(c, UsedByDTO{Count: len(routes), Routes: routes}))
}

// MergePatcher applies an RFC 7396 merge patch body onto the current input.
type MergePatcher struct{}

// Apply returns the merged input, rejecting malformed bodies and unknown fields with a JSON Pointer.
func (MergePatcher) Apply(w http.ResponseWriter, r *http.Request, current ConnectionInput) (ConnectionInput, error) {
	patch, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(api.BodyLimitDefault)))
	if err != nil {
		return ConnectionInput{}, api.JSONErrorMapper{}.Map(err)
	}
	base, err := json.Marshal(current)
	if err != nil {
		return ConnectionInput{}, err
	}
	merged, err := jsonpatch.MergePatch(base, patch)
	if err != nil {
		return ConnectionInput{}, api.JSONErrorMapper{}.Map(err)
	}
	var out ConnectionInput
	dec := json.NewDecoder(bytes.NewReader(merged))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return ConnectionInput{}, api.JSONErrorMapper{}.Map(err)
	}
	return out, nil
}

// DeleteHandler serves DELETE /api/v1/connections/{id}.
type DeleteHandler struct{ d Deps }

// Route implements api.Handler.
func (h *DeleteHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodDelete, Pattern: string(PathItem), Permission: PermWrite, Object: h.d.Object, Mutating: true, AuditAction: connections.ActionDeleted}
}

// Serve implements api.Handler.
func (h *DeleteHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	id, err := RouteID{}.Of(r)
	if err != nil {
		return err
	}
	if err := h.d.Service.Delete(r.Context(), id); err != nil {
		return err
	}
	return h.d.Responder.NoContent(w)
}
