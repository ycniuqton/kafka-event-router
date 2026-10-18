package routes

import (
	"context"
	"log/slog"
	"sync"

	"github.com/eventrouter/router/internal/platform/logging"
)

// ReplayPushEvent is the WS /events frame type carrying replay progress.
type ReplayPushEvent string

const PushReplayChanged ReplayPushEvent = "route.replay_changed"

// ReplayProgress is one replay status change of a route owned by TeamID (push visibility).
type ReplayProgress struct {
	TeamID TeamID
	Replay Replay
}

// ReplayProgressPublisher pushes replay progress to browsers (monitoring's events hub).
type ReplayProgressPublisher interface {
	Publish(ctx context.Context, p ReplayProgress)
}

// NopReplayProgress drops progress (no push channel configured); clients poll the status endpoint.
type NopReplayProgress struct{}

func (NopReplayProgress) Publish(context.Context, ReplayProgress) {}

// OpenReplays lists every replay that has not finished (route-runtime's route_replays).
type OpenReplays interface {
	Open(ctx context.Context) ([]Replay, error)
}

// ReplayHints yields the ids of routes whose replay changed (route-runtime's router_runtime NOTIFY).
type ReplayHints interface {
	Subscribe() (<-chan RouteID, func())
}

// RouteTeams resolves the owning team of a route.
type RouteTeams interface {
	Get(ctx context.Context, id RouteID) (Route, error)
}

// ReplayProgressRelay turns replay change hints into progress pushes: open replays are read from the
// runtime, a replay that left the open set is read once more by id for its final status.
type ReplayProgressRelay struct {
	Hints    ReplayHints
	Open     OpenReplays
	Resetter OffsetResetter
	Routes   RouteTeams
	Out      ReplayProgressPublisher
	Log      *slog.Logger

	mu   sync.Mutex
	last map[RouteID]Replay
}

// Run relays hints until ctx ends.
func (r *ReplayProgressRelay) Run(ctx context.Context) {
	hints, cancel := r.Hints.Subscribe()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case id, ok := <-hints:
			if !ok {
				return
			}
			if err := r.Handle(ctx, id); err != nil {
				r.Log.Warn("routes: replay progress relay failed", string(logging.AttrRouteID), string(id), string(logging.AttrError), err)
			}
		}
	}
}

// Handle publishes the current status of the route's replay when it changed since the last push.
func (r *ReplayProgressRelay) Handle(ctx context.Context, id RouteID) error {
	current, ok, err := r.current(ctx, id)
	if err != nil || !ok {
		return err
	}
	if !r.changed(current) {
		return nil
	}
	route, err := r.Routes.Get(ctx, id)
	if err != nil {
		return err
	}
	r.Out.Publish(ctx, ReplayProgress{TeamID: route.TeamID, Replay: current})
	return nil
}

// current returns the route's open replay, or the final state of the replay last seen open.
func (r *ReplayProgressRelay) current(ctx context.Context, id RouteID) (Replay, bool, error) {
	open, err := r.Open.Open(ctx)
	if err != nil {
		return Replay{}, false, err
	}
	for _, rp := range open {
		if rp.RouteID == id {
			return rp, true, nil
		}
	}
	prev, seen := r.seen(id)
	if !seen {
		return Replay{}, false, nil
	}
	final, err := r.Resetter.Status(ctx, id, prev.ID)
	return final, err == nil, err
}

// changed records rp and reports whether its status differs from the last push.
func (r *ReplayProgressRelay) changed(rp Replay) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.last == nil {
		r.last = map[RouteID]Replay{}
	}
	prev, ok := r.last[rp.RouteID]
	if rp.Status.Finished() {
		delete(r.last, rp.RouteID)
	} else {
		r.last[rp.RouteID] = rp
	}
	return !ok || prev.ID != rp.ID || prev.Status != rp.Status
}

func (r *ReplayProgressRelay) seen(id RouteID) (Replay, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rp, ok := r.last[id]
	return rp, ok
}
