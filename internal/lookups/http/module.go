// Package lookupshttp serves /api/v1/lookup-sources (CRUD, tests) and the workload-only spec endpoint.
package lookupshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	connhttp "github.com/eventrouter/router/internal/connections/http"
	"github.com/eventrouter/router/internal/lookups"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// Path names the routes and params.
type Path string

const (
	PathCollection Path = "/api/v1/lookup-sources"
	PathItem       Path = "/api/v1/lookup-sources/{id}"
	PathTestDraft  Path = "/api/v1/lookup-sources/test"
	PathTestSaved  Path = "/api/v1/lookup-sources/{id}/test"
	PathSpec       Path = "/internal/v1/lookup-sources/{id}/spec"
	ParamID        Path = "id"
)

// QueryParam names list query parameters.
type QueryParam string

const (
	QueryKind QueryParam = "kind"
	QueryEnv  QueryParam = "env"
	QueryText QueryParam = "q"
)

// Permissions reuse the connection permissions (design §6: no new catalog entries).
var (
	PermRead  = connhttp.PermRead
	PermWrite = connhttp.PermWrite
)

// ResourceLookupSource is the authorization object type of item routes.
const ResourceLookupSource rbac.ResourceType = "lookup_source"

// Deps are the collaborators of the HTTP module.
type Deps struct {
	Service   *lookups.Service
	Tests     lookups.TestService
	Responder *api.Responder
	Pages     *api.PageParser
	Recorder  audit.Recorder
	Authz     auth.Authorizer
	Object    auth.ObjectResolver
}

// withOwnerScope makes item routes team-scoped (the object carries the owner team).
func (d Deps) withOwnerScope() Deps {
	d.Object = SourceObject
	if d.Service != nil {
		d.Object = rbac.ObjectResolvers{}.Owned(ResourceLookupSource, string(ParamID), d.Service)
	}
	return d
}

// NewModule returns the public module.
func NewModule(d Deps) api.Module {
	d = d.withOwnerScope()
	ext := ProblemExtender{responder: d.Responder}
	return api.Modules{
		ext.Wrap(&ListHandler{d: d}), ext.Wrap(&GetHandler{d: d}), ext.Wrap(&CreateHandler{d: d}), ext.Wrap(&UpdateHandler{d: d}),
		ext.Wrap(&DeleteHandler{d: d}), ext.Wrap(&TestSavedHandler{d: d}), ext.Wrap(&TestDraftHandler{d: d}),
	}
}

// NewInternalModule returns the workload-only spec module.
func NewInternalModule(d Deps) api.Module { return api.Modules{&SpecHandler{d: d}} }

// RouteID parses the {id} path parameter.
type RouteID struct{}

// Of returns the id or a 404.
func (RouteID) Of(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return uuid.Nil, store.ErrNotFound
	}
	return id, nil
}

// SourceObject resolves the id-only authorization object (no owner lookup).
func SourceObject(r *http.Request) auth.ObjectRef {
	return auth.ObjectRef{Type: string(ResourceLookupSource), ID: chi.URLParam(r, string(ParamID))}
}

// TeamGuard enforces a permission on the target team of body-scoped routes (create, draft test).
type TeamGuard struct {
	authz auth.Authorizer
}

// Require returns the Authorizer's error when the caller lacks perm on team; without an Authorizer it allows.
func (g TeamGuard) Require(ctx context.Context, perm auth.Permission, team uuid.UUID) error {
	if g.authz == nil {
		return nil
	}
	return g.authz.Authorize(ctx, auth.PrincipalFrom(ctx), perm, auth.ObjectRef{Type: string(ResourceLookupSource), TeamID: team.String()})
}

// OwnerTeam resolves and authorizes the owner team of a body (same policy as connections).
type OwnerTeam struct {
	d Deps
}

// Resolve returns the team after checking the write permission on it.
func (o OwnerTeam) Resolve(ctx context.Context, owner *OwnerInput) (uuid.UUID, error) {
	var in *connhttp.OwnerInput
	if owner != nil {
		in = &connhttp.OwnerInput{TeamID: owner.TeamID}
	}
	team, err := connhttp.OwnerTeamPolicy{}.Resolve(auth.PrincipalFrom(ctx), in)
	if err != nil {
		return uuid.Nil, err
	}
	return team, TeamGuard{authz: o.d.Authz}.Require(ctx, PermWrite, team)
}

// ExtendedProblem is an RFC 7807 document with the in-use routes.
type ExtendedProblem struct {
	problem.Details
	Routes []lookups.RouteRef `json:"routes"`
}

// ProblemExtender renders lookups.InUseError with its own type and routes; other errors pass through.
type ProblemExtender struct {
	responder *api.Responder
}

// Wrap decorates a handler.
func (e ProblemExtender) Wrap(h api.Handler) api.Handler { return extendedHandler{inner: h, ext: e} }

// Write renders err when it is an InUseError and reports whether it did.
func (e ProblemExtender) Write(w http.ResponseWriter, r *http.Request, err error) bool {
	var inUse *lookups.InUseError
	if !errors.As(err, &inUse) {
		return false
	}
	d := problem.New(problem.TypeURI(inUse.ProblemTypeURI()), http.StatusConflict, inUse.Error())
	d.Instance, d.RequestID = r.URL.Path, api.RequestIDOf(r)
	w.Header().Set(string(problem.HeaderContentType), string(problem.MediaTypeProblemJSON))
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(ExtendedProblem{Details: d, Routes: inUse.Routes})
	return true
}

type extendedHandler struct {
	inner api.Handler
	ext   ProblemExtender
}

func (h extendedHandler) Route() api.RouteSpec { return h.inner.Route() }

func (h extendedHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	err := h.inner.Serve(w, r)
	if err != nil && h.ext.Write(w, r, err) {
		return nil
	}
	return err
}
