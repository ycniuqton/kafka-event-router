package main

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/library/resolver"
	"github.com/eventrouter/router/internal/library/testcases"
	"github.com/eventrouter/router/internal/library/testcaseshttp"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/routes/assembly"
	"github.com/eventrouter/router/internal/sandboxinfo"
	"github.com/eventrouter/router/pkg/engine/compile"
)

// testCasesWiring composes the saved test cases of transform-studio once per process: the HTTP modules
// and the routes validator port share one TestCaseService (runs compile in-process with pkg/engine).
// The JS sandbox and the library resolver are shared with library publish checks.
type testCasesWiring struct {
	once    sync.Once
	service *testcases.TestCaseService
	repo    *testcases.Repository
	scripts *testcases.LazySandboxScripts
	library *resolver.StoreLibraryResolver
}

var testCasesFeature = &testCasesWiring{}

// build creates the service on first use.
func (t *testCasesWiring) build(w *Wiring) *testcases.TestCaseService {
	t.once.Do(func() {
		salts := sandboxinfo.NewTeamSalts(w.Deps.Secrets, w.Identity.Teams)
		t.repo, t.library = testcases.NewRepository(w.Deps.DB), resolver.New(w.Deps.DB)
		t.scripts = &testcases.LazySandboxScripts{Salts: salts}
		runner := testcases.NewTestCaseRunner(testcases.RunnerDeps{Compiler: compile.NewDefaultCompiler(),
			Scripts: t.scripts, Salts: salts, Comparators: testcases.DefaultComparatorManager(), Library: t.library})
		targets := map[testcases.OwnerKind]testcases.TargetSource{
			testcases.OwnerRoute: testcases.RouteTargets{Routes: t.repo}, testcases.OwnerTransform: testcases.TransformTargets{Transforms: t.repo},
		}
		t.service = testcases.NewService(testcases.ServiceDeps{Store: t.repo, Routes: t.repo, Runner: runner, Limits: testcases.DefaultLimits, Targets: targets})
	})
	return t.service
}

// testCasesModule serves /api/v1/routes/{id}/test-cases and /api/v1/transforms/{id}/test-cases.
func (s *apiService) testCasesModule(transforms rbac.TeamOwner) api.Module {
	svc := testCasesFeature.build(s.wiring)
	masking := testcaseshttp.CaseMasking{Visibility: rawSampleAccess{w: s.wiring}, Masker: preview.NewTypePreservingMasker()}
	routes := testcaseshttp.NewModule(testcaseshttp.Deps{Service: svc, Owners: testCasesFeature.repo, Masking: masking, Responder: s.wiring.Responder})
	owned := testcaseshttp.Deps{Service: svc, Owners: transforms, Masking: masking, Responder: s.wiring.Responder}
	return api.Modules(append(routes.Handlers(), testcaseshttp.NewOwnerModule(owned, testcaseshttp.TransformOwner).Handlers()...))
}

// rawSampleAccess shows raw test case events only with sample.read_raw on the route's team (Viewers never).
type rawSampleAccess struct {
	w *Wiring
}

// RawAllowed implements testcaseshttp.RawVisibility; without access control values stay masked.
func (a rawSampleAccess) RawAllowed(ctx context.Context, teamID string) bool {
	team, err := uuid.Parse(teamID)
	if err != nil || a.w.Access == nil || a.w.Access.Manager == nil {
		return false
	}
	return a.w.Access.Manager.Decide(ctx, rbac.PermSampleReadRaw, rbac.Resource{Type: rbac.ResourceRoute, TeamID: team}).Allowed
}

// TestCasePort is routes' validation.TestCaseRunner over the shared service (routes' test-case validator).
func (w *Wiring) TestCasePort() testcases.RoutePort {
	return testcases.RoutePort{Service: testCasesFeature.build(w)}
}

func init() {
	routesPortContributors = append(routesPortContributors, contributeTestCasePort)
}

// contributeTestCasePort lets routes' TestCaseValidator run the saved test cases (deploy gate) and
// routes' compile checks resolve library steps and named lookup tables.
func contributeTestCasePort(w *Wiring, p *assembly.Ports) {
	p.TestCases = w.TestCasePort()
	p.LibraryResolver = testCasesFeature.library
}
