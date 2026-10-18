package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
	"github.com/eventrouter/router/internal/store"
)

var (
	// ErrNoActiveVersion is returned when a route without a deployed version is replayed.
	ErrNoActiveVersion = errors.New("route has no active version to replay")
	// ErrReplayInProgress is returned when the route already has an open replay.
	ErrReplayInProgress = errors.New("a replay of this route is already in progress")
	// ErrRouteNotFound is returned when the replayed route does not exist.
	ErrRouteNotFound = errors.New("route not found")
	// ErrReplayNotFound is returned for an unknown replay id.
	ErrReplayNotFound = errors.New("replay not found")
)

// ReplayRequest is one offset reset asked for by routes' POST /routes/{id}/replay.
type ReplayRequest struct {
	RouteID     string
	From        time.Time
	RequestedBy string
}

// ReplayService is route-runtime's offset-reset mechanism behind routes' OffsetResetter port: Reset records a
// pending replay that the leader's ReplayCoordinator carries out; Status reads its progress.
type ReplayService struct {
	Stores state.Stores
	Now    func() time.Time
}

// Reset validates the route and records a pending replay (one open replay per route).
func (s *ReplayService) Reset(ctx context.Context, req ReplayRequest) (state.Replay, error) {
	ok, err := s.Stores.Routes.Exists(ctx, req.RouteID)
	if err != nil {
		return state.Replay{}, err
	}
	if !ok {
		return state.Replay{}, errors.Join(ErrRouteNotFound, store.ErrNotFound)
	}
	active, err := s.Stores.Routes.ActiveVersion(ctx, req.RouteID)
	if err != nil {
		return state.Replay{}, err
	}
	if active == "" {
		return state.Replay{}, errors.Join(ErrNoActiveVersion, store.ErrConflict)
	}
	rp, err := s.Stores.Replays.Create(ctx, state.Replay{RouteID: req.RouteID, From: req.From, RequestedBy: req.RequestedBy,
		Status: domain.ReplayPending, RequestedAt: s.Now()})
	if errors.Is(err, store.ErrConflict) {
		return state.Replay{}, errors.Join(ErrReplayInProgress, store.ErrConflict)
	}
	return rp, err
}

// Status returns one replay of the route.
func (s *ReplayService) Status(ctx context.Context, routeID, replayID string) (state.Replay, error) {
	rp, err := s.Stores.Replays.Get(ctx, routeID, replayID)
	if errors.Is(err, store.ErrNotFound) {
		return state.Replay{}, errors.Join(ErrReplayNotFound, store.ErrNotFound)
	}
	return rp, err
}
