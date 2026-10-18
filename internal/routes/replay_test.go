package routes_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/routestest"
)

// replayWorld is a route core with one editor principal on a uuid team.
type replayWorld struct {
	core     *routes.Core
	resetter *routestest.Replays
	manager  *routes.ReplayManager
	ctx      context.Context
	team     routes.TeamID
	now      time.Time
}

func newReplayWorld(t *testing.T) *replayWorld {
	t.Helper()
	team := uuid.New()
	db := routestest.NewDB(routes.TeamID(team.String()), "payments")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	core := &routes.Core{Repos: db.Repositories(), Tx: routestest.Tx{DB: db}, Validator: passAll{}, Events: &routestest.Publisher{},
		Guard: routes.TeamRoleWriteGuard{}, Log: log, Views: routes.SummaryBuilder{Log: log}}
	w := &replayWorld{core: core, resetter: routestest.NewReplays(), team: routes.TeamID(team.String()),
		now: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)}
	w.manager = routes.NewReplayManager(core, w.resetter, routes.ReplayRules{})
	w.manager.Now = func() time.Time { return w.now }
	w.ctx = auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New(),
		Teams: []auth.TeamRole{{TeamID: team, Role: auth.RoleEditor}}})
	return w
}

// route creates a route, deployed when deployed is true.
func (w *replayWorld) route(t *testing.T, deployed bool) routes.RouteID {
	t.Helper()
	r, err := w.core.Repos.Routes.Create(w.ctx, routes.Route{TeamID: w.team, Name: uuid.NewString(), DesiredState: routes.DesiredRunning, Replicas: 1, Draft: []byte(`{"name":"r"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if !deployed {
		return r.ID
	}
	err = w.core.Tx.WithinTx(w.ctx, func(ctx context.Context) error {
		_, err := routes.VersionActivator{Repos: w.core.Repos}.Activate(ctx, routes.ActivationPlan{Route: r, Config: r.Draft, Origin: routes.OriginDeploy})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

func (w *replayWorld) request(from time.Time, confirm bool) routes.ReplayRequest {
	return routes.ReplayRequest{From: &from, Confirm: confirm}
}

func TestReplayRulesRejectMissingConfirmAndBadTimes(t *testing.T) {
	w := newReplayWorld(t)
	id := w.route(t, true)
	cases := map[string]struct {
		req   routes.ReplayRequest
		field string
	}{
		"no confirm": {w.request(w.now.Add(-time.Hour), false), string(routes.PathConfirm)},
		"no from":    {routes.ReplayRequest{Confirm: true}, string(routes.PathFrom)},
		"future":     {w.request(w.now.Add(time.Minute), true), string(routes.PathFrom)},
		"too old":    {w.request(w.now.Add(-8*24*time.Hour), true), string(routes.PathFrom)},
	}
	for name, c := range cases {
		_, err := w.manager.Request(w.ctx, id, c.req)
		var v *api.ValidationError
		if !errors.As(err, &v) || len(v.Errors) != 1 || v.Errors[0].Path != c.field {
			t.Errorf("%s: want 422 at %s, got %v", name, c.field, err)
		}
	}
	if w.resetter.Len() != 0 {
		t.Fatal("rejected requests must not reach the runtime")
	}
}

func TestReplayRequestRecordsPendingReplay(t *testing.T) {
	w := newReplayWorld(t)
	id := w.route(t, true)
	rp, err := w.manager.Request(w.ctx, id, w.request(w.now.Add(-time.Hour), true))
	if err != nil || rp.Status != routes.ReplayPending || rp.RouteID != id || !rp.From.Equal(w.now.Add(-time.Hour)) {
		t.Fatalf("replay %+v err %v", rp, err)
	}
	if _, err := w.manager.Request(w.ctx, id, w.request(w.now.Add(-time.Hour), true)); !errors.Is(err, routes.ErrReplayInProgress) {
		t.Fatalf("second open replay: %v", err)
	}
	got, err := w.manager.Status(w.ctx, id, rp.ID)
	if err != nil || got.ID != rp.ID {
		t.Fatalf("status %+v %v", got, err)
	}
	if _, err := w.manager.Status(w.ctx, id, routes.ReplayID(uuid.NewString())); !errors.Is(err, routes.ErrReplayNotFound) {
		t.Fatalf("unknown replay: %v", err)
	}
}

func TestReplayGuards(t *testing.T) {
	w := newReplayWorld(t)
	req := w.request(w.now.Add(-time.Hour), true)
	if _, err := w.manager.Request(w.ctx, w.route(t, false), req); !errors.Is(err, routes.ErrNotDeployed) {
		t.Fatalf("draft route: %v", err)
	}
	if _, err := w.manager.Request(w.ctx, routes.RouteID(uuid.NewString()), req); !errors.Is(err, routes.ErrRouteNotFound) {
		t.Fatalf("unknown route: %v", err)
	}
	viewer := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.PrincipalUser, UserID: uuid.New()})
	if _, err := w.manager.Request(viewer, w.route(t, true), req); !errors.Is(err, routes.ErrForbiddenTeam) {
		t.Fatalf("other team: %v", err)
	}
	off := routes.NewReplayManager(w.core, nil, routes.ReplayRules{})
	off.Now = w.manager.Now
	if _, err := off.Request(w.ctx, w.route(t, true), req); !errors.Is(err, routes.ErrReplayUnavailable) {
		t.Fatalf("no runtime: %v", err)
	}
}

// pushes collects relayed progress.
type pushes struct {
	got []routes.ReplayProgress
}

func (p *pushes) Publish(_ context.Context, rp routes.ReplayProgress) { p.got = append(p.got, rp) }

func TestReplayProgressRelayPushesEachStatusOnce(t *testing.T) {
	w := newReplayWorld(t)
	id := w.route(t, true)
	rp, err := w.manager.Request(w.ctx, id, w.request(w.now.Add(-time.Hour), true))
	if err != nil {
		t.Fatal(err)
	}
	out := &pushes{}
	relay := &routes.ReplayProgressRelay{Open: w.resetter, Resetter: w.resetter, Routes: w.core.Repos.Routes, Out: out}
	ctx := context.Background()
	steps := []routes.ReplayStatus{routes.ReplayPending, routes.ReplayPending, routes.ReplayStopping, routes.ReplayResetting, routes.ReplayDone, routes.ReplayDone}
	for _, s := range steps {
		w.resetter.Set(rp.ID, s)
		if err := relay.Handle(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	want := []routes.ReplayStatus{routes.ReplayPending, routes.ReplayStopping, routes.ReplayResetting, routes.ReplayDone}
	if len(out.got) != len(want) {
		t.Fatalf("pushes %+v", out.got)
	}
	for i, p := range out.got {
		if p.Replay.Status != want[i] || p.TeamID != w.team {
			t.Errorf("push %d: %+v", i, p)
		}
	}
}
