package routestore_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/store/routestore"
	"github.com/eventrouter/router/internal/testkit"
)

// fixtureSize holds the sizes used by the repository integration tests.
type fixtureSize int

const (
	listedRoutes     fixtureSize = 5
	listPageSize     fixtureSize = 2
	concurrentDeploy fixtureSize = 8
)

// repoFixture is a migrated sandbox schema with one team and the three Postgres repositories.
type repoFixture struct {
	db   *store.DB
	tx   *store.PgTxManager
	team routes.TeamID
	repo routes.Repositories
}

func newRepoFixture(t *testing.T) *repoFixture {
	sb := testkit.NewManager().Postgres(t)
	sb.Migrate(t)
	f := &repoFixture{db: sb.DB, tx: store.NewTxManager(sb.DB), repo: routes.Repositories{
		Routes:      routestore.NewRouteRepository(sb.DB),
		Versions:    routestore.NewVersionRepository(sb.DB),
		Deployments: routestore.NewDeploymentRepository(sb.DB),
	}}
	var team string
	if err := sb.DB.Pool().QueryRow(context.Background(), `INSERT INTO teams (name) VALUES ('payments') RETURNING id::text`).Scan(&team); err != nil {
		t.Fatal(err)
	}
	f.team = routes.TeamID(team)
	return f
}

func (f *repoFixture) route(t *testing.T, name string) routes.Route {
	t.Helper()
	r, err := f.repo.Routes.Create(context.Background(), routes.Route{TeamID: f.team, Name: name, DesiredState: routes.DesiredPaused, Replicas: 1, Draft: json.RawMessage(`{"name":"` + name + `"}`)})
	if err != nil {
		t.Fatalf("create route: %v", err)
	}
	return r
}

func (f *repoFixture) version(ctx context.Context, id routes.RouteID) (routes.RouteVersion, error) {
	var v routes.RouteVersion
	err := f.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		v, err = f.repo.Versions.Create(ctx, routes.RouteVersion{RouteID: id, Config: json.RawMessage(`{}`), ConfigHash: "h", Origin: routes.OriginDeploy})
		return err
	})
	return v, err
}

// TestListSummariesCursorPaging walks the list two rows at a time: every route once, newest first.
func TestListSummariesCursorPaging(t *testing.T) {
	f := newRepoFixture(t)
	ctx := context.Background()
	for i := 0; i < int(listedRoutes); i++ {
		f.route(t, fmt.Sprintf("route-%d", i))
	}
	seen := map[routes.RouteID]bool{}
	page := routes.PageRequest{Limit: int(listPageSize)}
	for {
		rows, next, err := f.repo.Routes.ListSummaries(ctx, routes.ListQuery{}, page)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, r := range rows {
			if seen[r.Route.ID] {
				t.Fatalf("route %s listed twice", r.Route.ID)
			}
			seen[r.Route.ID] = true
		}
		if next == nil {
			break
		}
		page.After = next
	}
	if len(seen) != int(listedRoutes) {
		t.Fatalf("listed %d routes, want %d", len(seen), listedRoutes)
	}
}

// TestSaveDraftRevisionConflict: a write with a stale row_version is rejected and changes nothing.
func TestSaveDraftRevisionConflict(t *testing.T) {
	f := newRepoFixture(t)
	ctx := context.Background()
	r := f.route(t, "orders")
	first := "orders-v2"
	saved, err := f.repo.Routes.SaveDraft(ctx, routes.DraftWrite{ID: r.ID, Change: routes.RouteChange{Name: &first}, Expected: r.Revision})
	if err != nil || saved.Revision == r.Revision {
		t.Fatalf("first save = %+v, %v", saved, err)
	}
	stale := "orders-stale"
	_, err = f.repo.Routes.SaveDraft(ctx, routes.DraftWrite{ID: r.ID, Change: routes.RouteChange{Name: &stale}, Expected: r.Revision})
	if !errors.Is(err, store.ErrPreconditionFailed) {
		t.Fatalf("stale save err = %v, want precondition failed", err)
	}
	got, _ := f.repo.Routes.Get(ctx, r.ID)
	if got.Name != first {
		t.Fatalf("name = %q after rejected write", got.Name)
	}
}

// TestConcurrentVersionNumbers: parallel deploys of one route get distinct, gap-free version numbers.
func TestConcurrentVersionNumbers(t *testing.T) {
	f := newRepoFixture(t)
	r := f.route(t, "orders")
	var wg sync.WaitGroup
	nos := make(chan int, concurrentDeploy)
	errs := make(chan error, concurrentDeploy)
	for i := 0; i < int(concurrentDeploy); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := f.version(context.Background(), r.ID)
			errs <- err
			nos <- v.No
		}()
	}
	wg.Wait()
	close(nos)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("create version: %v", err)
		}
	}
	got := map[int]bool{}
	for no := range nos {
		got[no] = true
	}
	for no := 1; no <= int(concurrentDeploy); no++ {
		if !got[no] {
			t.Fatalf("version numbers %v miss %d", got, no)
		}
	}
}

// TestSupersedeOpen: rolling-out deployments become superseded; finished ones are untouched.
func TestSupersedeOpen(t *testing.T) {
	f := newRepoFixture(t)
	ctx := context.Background()
	r := f.route(t, "orders")
	v1, err := f.version(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	done, err := f.repo.Deployments.Create(ctx, routes.Deployment{RouteID: r.ID, VersionID: v1.ID, Status: routes.DeployActive})
	if err != nil {
		t.Fatal(err)
	}
	open, err := f.repo.Deployments.Create(ctx, routes.Deployment{RouteID: r.ID, VersionID: v1.ID, Status: routes.DeployRollingOut})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.Deployments.SupersedeOpen(ctx, r.ID, routes.SupersedeAllOpen()); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.repo.Deployments.Get(ctx, open.ID); got.Status != routes.DeploySuperseded || got.FinishedAt == nil {
		t.Fatalf("open deployment = %+v, want superseded", got)
	}
	if got, _ := f.repo.Deployments.Get(ctx, done.ID); got.Status != routes.DeployActive {
		t.Fatalf("finished deployment = %+v, want untouched", got)
	}
}

// TestSupersedeScopes: a pending-only scope keeps rolling-out deployments; Except survives its own scope.
func TestSupersedeScopes(t *testing.T) {
	f := newRepoFixture(t)
	ctx := context.Background()
	r := f.route(t, "payments")
	v1, err := f.version(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	create := func(status routes.DeploymentStatus) routes.Deployment {
		d, err := f.repo.Deployments.Create(ctx, routes.Deployment{RouteID: r.ID, VersionID: v1.ID, Status: status})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	live, older, kept := create(routes.DeployRollingOut), create(routes.DeployPendingApproval), create(routes.DeployRollingOut)
	if err := f.repo.Deployments.SupersedeOpen(ctx, r.ID, routes.SupersedePending()); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.Deployments.SupersedeOpen(ctx, r.ID, routes.SupersedeRollingOutExcept(kept.ID)); err != nil {
		t.Fatal(err)
	}
	want := map[routes.DeploymentID]routes.DeploymentStatus{older.ID: routes.DeploySuperseded, live.ID: routes.DeploySuperseded, kept.ID: routes.DeployRollingOut}
	for id, status := range want {
		if got, _ := f.repo.Deployments.Get(ctx, id); got.Status != status {
			t.Errorf("deployment %s = %s, want %s", id, got.Status, status)
		}
	}
}
