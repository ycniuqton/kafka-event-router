package connhttp

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/connections"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// Path names the connection routes and params.
type Path string

const (
	PathCollection Path = "/api/v1/connections"
	PathItem       Path = "/api/v1/connections/{id}"
	PathTestDraft  Path = "/api/v1/connections/test"
	PathTestSaved  Path = "/api/v1/connections/{id}/test"
	PathTopics     Path = "/api/v1/connections/{id}/topics"
	PathSpec       Path = "/internal/v1/connections/{id}/spec"
	ParamID        Path = "id"
)

// QueryParam names list/topic query parameters.
type QueryParam string

const (
	QueryEnv             QueryParam = "env"
	QueryStatus          QueryParam = "status"
	QueryText            QueryParam = "q"
	QueryIncludeInternal QueryParam = "includeInternal"
)

// Permissions of the connection routes (keys follow the access-control catalog).
var (
	PermRead        = auth.Permission{Key: "connection.read", Access: auth.AccessRead}
	PermWrite       = auth.Permission{Key: "connection.write", Access: auth.AccessWrite}
	PermTopicCreate = auth.Permission{Key: "topic.create", Access: auth.AccessWrite}
)

// ObjectConnection is the authorization object type of connection routes.
const ObjectConnection = string(audit.ObjectConnection)

// Deps are the collaborators of the HTTP module.
type Deps struct {
	Service   *connections.Service
	Tester    *connections.Tester
	Topics    *connections.TopicBrowser
	Creator   *connections.TopicCreator
	Resolver  *connections.SpecResolver
	Responder *api.Responder
	Pages     *api.PageParser
	Recorder  audit.Recorder
	Authz     auth.Authorizer
	Object    auth.ObjectResolver
}

// withOwnerScope makes item routes team-scoped: the authorization object carries the owner team (CN-108).
func (d Deps) withOwnerScope() Deps {
	d.Object = ConnectionObject
	if d.Service != nil {
		d.Object = rbac.ObjectResolvers{}.Owned(rbac.ResourceConnection, string(ParamID), d.Service)
	}
	return d
}

// NewModule returns the public /api/v1/connections module.
func NewModule(d Deps) api.Module {
	d = d.withOwnerScope()
	ext := ProblemExtender{responder: d.Responder}
	return api.Modules{
		ext.Wrap(&ListHandler{d: d}), ext.Wrap(&GetHandler{d: d}), ext.Wrap(&CreateHandler{d: d}),
		ext.Wrap(&UpdateHandler{d: d}), ext.Wrap(&DeleteHandler{d: d}), ext.Wrap(&TestSavedHandler{d: d}),
		ext.Wrap(&TestDraftHandler{d: d}), ext.Wrap(&TopicsHandler{d: d}), ext.Wrap(&CreateTopicHandler{d: d}),
	}
}

// NewInternalModule returns the workload-only spec module.
func NewInternalModule(d Deps) api.Module {
	return api.Modules{&SpecHandler{d: d}}
}

// ErrBadID is returned for a malformed connection id.
var ErrBadID = store.ErrNotFound

// RouteID parses the {id} path parameter.
type RouteID struct{}

// Of returns the connection id or a 404.
func (RouteID) Of(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return uuid.Nil, ErrBadID
	}
	return id, nil
}

// ConnectionObject resolves the id-only authorization object of item routes (no owner lookup).
func ConnectionObject(r *http.Request) auth.ObjectRef {
	return auth.ObjectRef{Type: ObjectConnection, ID: chi.URLParam(r, string(ParamID))}
}

// ProblemStatus maps connection problem types to HTTP statuses.
var ProblemStatus = map[connections.ProblemType]int{
	connections.ProblemConnectionInUse:     http.StatusConflict,
	connections.ProblemTopicExists:         http.StatusConflict,
	connections.ProblemTopicCreateDisabled: http.StatusForbidden,
	connections.ProblemClusterUnavailable:  http.StatusBadGateway,
}

// ExtendedProblem is an RFC 7807 document with the connection extension members.
type ExtendedProblem struct {
	problem.Details
	connections.ProblemExtras
}

// ProblemExtender renders connections.ExtendedError with its own type and members; other errors pass through.
type ProblemExtender struct {
	responder *api.Responder
}

// Wrap decorates a handler.
func (e ProblemExtender) Wrap(h api.Handler) api.Handler {
	return extendedHandler{inner: h, ext: e}
}

// Write renders err when it is an ExtendedError and reports whether it did.
func (e ProblemExtender) Write(w http.ResponseWriter, r *http.Request, err error) bool {
	var ext connections.ExtendedError
	if !errors.As(err, &ext) {
		return false
	}
	status := ProblemStatus[ext.ProblemTypeURI()]
	d := problem.New(problem.TypeURI(ext.ProblemTypeURI()), status, ext.Error())
	d.Instance, d.RequestID = r.URL.Path, api.RequestIDOf(r)
	var pointed problem.Pointed
	if errors.As(err, &pointed) {
		d.Path = pointed.ProblemPath()
	}
	w.Header().Set(string(problem.HeaderContentType), string(problem.MediaTypeProblemJSON))
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ExtendedProblem{Details: d, ProblemExtras: ext.ProblemExtras()})
	return true
}

// extendedHandler applies the ProblemExtender to one handler.
type extendedHandler struct {
	inner api.Handler
	ext   ProblemExtender
}

// Route implements api.Handler.
func (h extendedHandler) Route() api.RouteSpec { return h.inner.Route() }

// Serve implements api.Handler.
func (h extendedHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	err := h.inner.Serve(w, r)
	if err != nil && h.ext.Write(w, r, err) {
		return nil
	}
	return err
}
