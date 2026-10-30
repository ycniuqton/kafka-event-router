package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/events"
	"github.com/eventrouter/router/internal/library"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/assembly"
	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/internal/runtime/changes"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/scheduler"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/store/routestore"
	"github.com/eventrouter/router/internal/store/runtimestore"
)

func init() {
	routesPortContributors = append(routesPortContributors, contributeRoutesP3Ports)
}

// contributeRoutesP3Ports gives routes the transform library (library ref validator) and route-runtime's
// offset reset (replay API).
func contributeRoutesP3Ports(w *Wiring, p *assembly.Ports) {
	p.Library = routesLibraryLookup{repo: library.NewRepository(w.Deps.DB)}
	p.Replays = routesOffsetResetter{}
}

// routesLibraryLookup resolves library objects by name through transform-studio's library repository
// (names are unique across teams; the route's team is not needed to resolve them).
type routesLibraryLookup struct {
	repo *library.Repository
}

// libraryKinds maps routes' library kinds to the library's content kinds (only their tables are used).
var libraryKinds = map[validation.LibraryKind]library.ContentKind{
	validation.LibraryTransform:   library.TransformKind{},
	validation.LibraryLookupTable: library.LookupTableKind{},
}

// Latest implements validation.LibraryLookup.
func (l routesLibraryLookup) Latest(ctx context.Context, ref validation.LibraryRef) (int, bool, error) {
	it, err := l.repo.GetByName(ctx, libraryKinds[ref.Kind], ref.Name)
	if errors.Is(err, library.ErrNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return it.LatestVersion, it.LatestVersion > 0, nil
}

// routesOffsetResetter is routes.OffsetResetter over route-runtime's ReplayService (read at call time,
// since route-runtime is built after routes).
type routesOffsetResetter struct{}

// Reset implements routes.OffsetResetter.
func (routesOffsetResetter) Reset(ctx context.Context, req routes.OffsetResetRequest) (routes.Replay, error) {
	svc := runtimeFeature.replays
	if svc == nil {
		return routes.Replay{}, routes.ErrReplayUnavailable
	}
	by := ""
	if req.RequestedBy != nil {
		by = req.RequestedBy.String()
	}
	rp, err := svc.Reset(ctx, scheduler.ReplayRequest{RouteID: string(req.RouteID), From: req.From, RequestedBy: by})
	if err != nil {
		return routes.Replay{}, replayErrors.Map(err)
	}
	return replayView(rp), nil
}

// Status implements routes.OffsetResetter.
func (routesOffsetResetter) Status(ctx context.Context, route routes.RouteID, id routes.ReplayID) (routes.Replay, error) {
	svc := runtimeFeature.replays
	if svc == nil {
		return routes.Replay{}, routes.ErrReplayUnavailable
	}
	rp, err := svc.Status(ctx, string(route), string(id))
	if err != nil {
		return routes.Replay{}, replayErrors.Map(err)
	}
	return replayView(rp), nil
}

// ReplayErrorMap translates route-runtime replay errors into routes problems.
type ReplayErrorMap []struct {
	From error
	To   error
}

var replayErrors = ReplayErrorMap{
	{scheduler.ErrReplayInProgress, routes.ErrReplayInProgress},
	{scheduler.ErrNoActiveVersion, routes.ErrNotDeployed},
	{scheduler.ErrRouteNotFound, routes.ErrRouteNotFound},
	{scheduler.ErrReplayNotFound, routes.ErrReplayNotFound},
	{store.ErrNotFound, routes.ErrReplayNotFound},
}

// Map returns the routes problem of err, or err unchanged.
func (m ReplayErrorMap) Map(err error) error {
	for _, e := range m {
		if errors.Is(err, e.From) {
			return e.To
		}
	}
	return err
}

// replayView converts a route_replays row to the routes API view.
func replayView(rp state.Replay) routes.Replay {
	return routes.Replay{ID: routes.ReplayID(rp.ID), RouteID: routes.RouteID(rp.RouteID), Status: routes.ReplayStatus(rp.Status),
		From: rp.From, RequestedAt: rp.RequestedAt, StartedAt: rp.StartedAt, FinishedAt: rp.FinishedAt, Error: rp.Error, Partitions: rp.Partitions}
}

// replayProgressWiring starts the replay progress relay once per process.
type replayProgressWiring struct {
	once sync.Once
}

var replayProgressFeature = &replayProgressWiring{}

// start LISTENs on router_runtime (own connection) and pushes route.replay_changed to the local hub.
func (r *replayProgressWiring) start(w *Wiring) {
	r.once.Do(func() {
		pushFeature.build(w)
		ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		broker := changes.NewBroker()
		go changes.PgListener{Notifications: w.Deps.DB.Notifications(), Broker: broker}.Run(ctx)
		relay := &routes.ReplayProgressRelay{Hints: replayHints{broker: broker}, Open: openReplays{stores: runtimestore.New(w.Deps.DB, w.Tx)},
			Resetter: routesOffsetResetter{}, Routes: routestore.NewRouteRepository(w.Deps.DB),
			Out: replayPush{out: pushFeature.hub, factory: events.EventFactory{Now: time.Now}, w: w}, Log: w.Deps.Log}
		go relay.Run(ctx)
	})
}

// replayHints filters router_runtime hints down to replay changes.
type replayHints struct {
	broker *changes.Broker
}

// Subscribe implements routes.ReplayHints.
func (h replayHints) Subscribe() (<-chan routes.RouteID, func()) {
	in, cancel := h.broker.Subscribe()
	out := make(chan routes.RouteID, changes.SubscriberBuffer)
	go func() {
		defer close(out)
		for hint := range in {
			if hint.Kind == domain.ChangeReplay && hint.RouteID != "" {
				out <- routes.RouteID(hint.RouteID)
			}
		}
	}()
	return out, cancel
}

// openReplays lists route-runtime's open replays.
type openReplays struct {
	stores state.Stores
}

// Open implements routes.OpenReplays.
func (o openReplays) Open(ctx context.Context) ([]routes.Replay, error) {
	rows, err := o.stores.Replays.Open(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]routes.Replay, 0, len(rows))
	for _, rp := range rows {
		out = append(out, replayView(rp))
	}
	return out, nil
}

// replayPush publishes replay progress on the local WS hub, visible to the route's team.
type replayPush struct {
	out     events.Publisher
	factory events.EventFactory
	w       *Wiring
}

// Publish implements routes.ReplayProgressPublisher.
func (p replayPush) Publish(ctx context.Context, rp routes.ReplayProgress) {
	if _, err := uuid.Parse(string(rp.TeamID)); err != nil {
		return
	}
	e, err := p.factory.New(events.PushEventType(routes.PushReplayChanged), string(rp.TeamID), rp.Replay)
	if err == nil {
		err = p.out.Publish(ctx, e)
	}
	if err != nil {
		p.w.Deps.Log.Warn("routes: replay progress push failed", string(logging.AttrError), err)
	}
}
