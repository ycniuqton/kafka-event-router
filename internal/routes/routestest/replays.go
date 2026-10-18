package routestest

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/routes"
)

// Replays is an in-memory routes.OffsetResetter and routes.OpenReplays with route-runtime's rule of
// one open replay per route; tests move replays with Set.
type Replays struct {
	mu   sync.Mutex
	rows map[routes.ReplayID]routes.Replay
}

// NewReplays returns an empty store.
func NewReplays() *Replays {
	return &Replays{rows: map[routes.ReplayID]routes.Replay{}}
}

// Reset records a pending replay.
func (f *Replays) Reset(_ context.Context, req routes.OffsetResetRequest) (routes.Replay, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rp := range f.rows {
		if rp.RouteID == req.RouteID && !rp.Status.Finished() {
			return routes.Replay{}, routes.ErrReplayInProgress
		}
	}
	rp := routes.Replay{ID: routes.ReplayID(uuid.NewString()), RouteID: req.RouteID, Status: routes.ReplayPending, From: req.From, RequestedAt: req.From}
	f.rows[rp.ID] = rp
	return rp, nil
}

// Status returns one replay of route.
func (f *Replays) Status(_ context.Context, route routes.RouteID, id routes.ReplayID) (routes.Replay, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rp, ok := f.rows[id]
	if !ok || rp.RouteID != route {
		return routes.Replay{}, routes.ErrReplayNotFound
	}
	return rp, nil
}

// Open lists unfinished replays.
func (f *Replays) Open(context.Context) ([]routes.Replay, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []routes.Replay{}
	for _, rp := range f.rows {
		if !rp.Status.Finished() {
			out = append(out, rp)
		}
	}
	return out, nil
}

// Set moves a replay to status s.
func (f *Replays) Set(id routes.ReplayID, s routes.ReplayStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rp := f.rows[id]
	rp.Status = s
	f.rows[id] = rp
}

// Len counts recorded replays.
func (f *Replays) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows)
}
