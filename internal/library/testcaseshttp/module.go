// Package testcaseshttp serves saved test cases of a route (/api/v1/routes/{id}/test-cases) and of a library
// transform (/api/v1/transforms/{id}/test-cases): list, create, patch with If-Match, delete and /run.
package testcaseshttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/library/testcases"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/rbac"
)

// Path names the test case routes.
type Path string

const (
	PathCollection          Path = "/api/v1/routes/{id}/test-cases"
	PathItem                Path = "/api/v1/routes/{id}/test-cases/{caseId}"
	PathRun                 Path = "/api/v1/routes/{id}/test-cases/run"
	PathTransformCollection Path = "/api/v1/transforms/{id}/test-cases"
	PathTransformItem       Path = "/api/v1/transforms/{id}/test-cases/{caseId}"
	PathTransformRun        Path = "/api/v1/transforms/{id}/test-cases/run"
	ParamRouteID            Path = "id"
	ParamCaseID             Path = "caseId"
)

// OwnerRoutes binds one owner kind to its paths, RBAC resource and permissions.
type OwnerRoutes struct {
	Kind       testcases.OwnerKind
	Collection Path
	Item       Path
	Run        Path
	Resource   rbac.ResourceType
	Read       auth.Permission
	Write      auth.Permission
}

// RouteOwner serves route cases: readable and runnable with route.read, changed with testcase.write.
var RouteOwner = OwnerRoutes{Kind: testcases.OwnerRoute, Collection: PathCollection, Item: PathItem, Run: PathRun,
	Resource: rbac.ResourceRoute, Read: rbac.PermRouteRead.Auth(), Write: rbac.PermTestCaseWrite.Auth()}

// TransformOwner serves library transform cases: open read (transform.read), changed with library.write.
var TransformOwner = OwnerRoutes{Kind: testcases.OwnerTransform, Collection: PathTransformCollection, Item: PathTransformItem,
	Run: PathTransformRun, Resource: rbac.ResourceType(testcases.OwnerTransform), Read: rbac.PermTransformRead.Auth(),
	Write: rbac.PermLibraryWrite.Auth()}

// ObjectTestCase is the audit object type of test cases.
const ObjectTestCase audit.ObjectType = "test_case"

// Audit actions of test case writes.
const (
	ActionCreated audit.Action = "test_case.created"
	ActionUpdated audit.Action = "test_case.updated"
	ActionDeleted audit.Action = "test_case.deleted"
)

// AuditActions lists every action of the module (for the audit catalog).
func AuditActions() []audit.Action {
	return []audit.Action{ActionCreated, ActionUpdated, ActionDeleted}
}

// Deps are the collaborators of the module; Owners resolves the owner team of the module's owner kind.
type Deps struct {
	Service   *testcases.TestCaseService
	Owners    rbac.TeamOwner
	Masking   CaseMasking
	Responder *api.Responder
}

// NewModule returns the route test case handlers.
func NewModule(d Deps) api.Module {
	return NewOwnerModule(d, RouteOwner)
}

// NewOwnerModule returns the test case handlers of one owner kind.
func NewOwnerModule(d Deps, o OwnerRoutes) api.Module {
	return api.Modules{&ListHandler{d: d, o: o}, &CreateHandler{d: d, o: o}, &PatchHandler{d: d, o: o},
		&DeleteHandler{d: d, o: o}, &RunHandler{d: d, o: o}}
}

// OwnerObject resolves the owner (and its team) of a request.
func (d Deps) OwnerObject(o OwnerRoutes) auth.ObjectResolver {
	return rbac.ObjectResolvers{}.Owned(o.Resource, string(ParamRouteID), d.Owners)
}

// Params reads typed path parameters of one owner kind; malformed ids are unknown objects (404).
type Params struct {
	Kind testcases.OwnerKind
}

// Owner returns the owner of the request.
func (p Params) Owner(r *http.Request) (testcases.Owner, error) {
	id, err := uuid.Parse(chi.URLParam(r, string(ParamRouteID)))
	if err != nil {
		return testcases.Owner{}, testcases.ErrNotFound
	}
	return testcases.Owner{Kind: p.Kind, ID: id}, nil
}

// CaseID returns the {caseId} parameter.
func (Params) CaseID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, string(ParamCaseID)))
	if err != nil {
		return uuid.Nil, testcases.ErrNotFound
	}
	return id, nil
}

// Auditor records test case writes in the request's audit scope (name and outcome, never event data).
type Auditor struct{}

// Record writes one entry; requests without a scope (tests) record nothing.
func (Auditor) Record(r *http.Request, action audit.Action, c testcases.TestCase, team string) error {
	scope, ok := audit.FromContext(r.Context())
	if !ok {
		return nil
	}
	var teamID *uuid.UUID
	if id, err := uuid.Parse(team); err == nil {
		teamID = &id
	}
	return scope.Record(r.Context(), audit.Change{Action: action, Object: ObjectTestCase, ObjectID: c.ID.String(), TeamID: teamID,
		After: CaseAudit{RouteID: c.Owner.ID.String(), OwnerKind: string(c.Owner.Kind), Name: c.Name, BranchID: c.BranchID, ExpectedOutcome: string(c.ExpectedOutcome)}})
}

// CaseAudit is the audited summary of a case.
type CaseAudit struct {
	RouteID         string `json:"routeId"`
	OwnerKind       string `json:"ownerKind"`
	Name            string `json:"name"`
	BranchID        string `json:"branchId"`
	ExpectedOutcome string `json:"expectedOutcome"`
}
