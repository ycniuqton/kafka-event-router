package routes

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/problem"
)

// ReplayID identifies one replay request (route-runtime's route_replays row).
type ReplayID string

// ReplayStatus is the progress of a replay; it mirrors route-runtime's replay_status values.
type ReplayStatus string

const (
	ReplayPending   ReplayStatus = "pending"
	ReplayStopping  ReplayStatus = "stopping"
	ReplayResetting ReplayStatus = "resetting"
	ReplayDone      ReplayStatus = "done"
	ReplayFailed    ReplayStatus = "failed"
)

// Finished reports whether the replay reached a final status.
func (s ReplayStatus) Finished() bool { return s == ReplayDone || s == ReplayFailed }

// ActionRouteReplayed is audited when someone replays a route.
const ActionRouteReplayed = audit.ActionRouteReplayed

// Replay is the API view of one replay (GET /routes/{id}/replays/{replayId}).
type Replay struct {
	ID          ReplayID     `json:"replayId"`
	RouteID     RouteID      `json:"routeId"`
	Status      ReplayStatus `json:"status"`
	From        time.Time    `json:"from"`
	RequestedAt time.Time    `json:"requestedAt"`
	StartedAt   *time.Time   `json:"startedAt,omitempty"`
	FinishedAt  *time.Time   `json:"finishedAt,omitempty"`
	Error       string       `json:"error,omitempty"`
	Partitions  int          `json:"partitions"`
}

// ReplayRequest is the body of POST /routes/{id}/replay.
type ReplayRequest struct {
	From    *time.Time `json:"from"`
	Confirm bool       `json:"confirm"`
}

// OffsetResetRequest asks route-runtime to re-read the route's source from From.
type OffsetResetRequest struct {
	RouteID     RouteID
	From        time.Time
	RequestedBy *uuid.UUID
}

// OffsetResetter is route-runtime's replay mechanism (ReplayService + ReplayCoordinator): Reset records a
// pending replay the scheduler leader carries out, Status reads one replay of a route.
type OffsetResetter interface {
	Reset(ctx context.Context, req OffsetResetRequest) (Replay, error)
	Status(ctx context.Context, route RouteID, id ReplayID) (Replay, error)
}

// ReplayAuditView is the audited snapshot of a replay request.
type ReplayAuditView struct {
	ReplayID ReplayID  `json:"replayId"`
	From     time.Time `json:"from"`
}

var (
	// ErrReplayInProgress is returned when the route already has an open replay (409).
	ErrReplayInProgress = problem.NewError(problem.KindConflict, "A replay of this route is already running. Wait until it finishes.")
	// ErrReplayNotFound is returned for unknown replay ids (404).
	ErrReplayNotFound = problem.NewError(problem.KindNotFound, "This replay does not exist.")
	// ErrReplayUnavailable is returned when the route runtime is not running in this API (503).
	ErrReplayUnavailable = problem.NewError(problem.KindUnavailable, "Replay is not available: the route runtime is disabled.")
)

// ReplayManager owns POST /routes/{id}/replay and its status (flow 5.14); the offset reset itself is
// route-runtime's, reached through OffsetResetter. Progress (from the pending insert on) reaches WS /events
// through ReplayProgressRelay.
type ReplayManager struct {
	*Core
	Resetter OffsetResetter
	Rules    ReplayRules
	Now      func() time.Time
}

// NewReplayManager builds the manager; a nil resetter answers 503.
func NewReplayManager(core *Core, resetter OffsetResetter, rules ReplayRules) *ReplayManager {
	return &ReplayManager{Core: core, Resetter: resetter, Rules: rules, Now: time.Now}
}

// Request validates the request and records the pending replay with its audit entry.
func (m *ReplayManager) Request(ctx context.Context, id RouteID, req ReplayRequest) (Replay, error) {
	if err := m.Rules.Check(req, m.Now()); err != nil {
		return Replay{}, err
	}
	route, actor, err := m.loadWritable(ctx, id)
	if err != nil {
		return Replay{}, err
	}
	if err := m.guard(route); err != nil {
		return Replay{}, err
	}
	var rp Replay
	err = m.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if rp, err = m.Resetter.Reset(ctx, OffsetResetRequest{RouteID: id, From: req.From.UTC(), RequestedBy: actor.UserID}); err != nil {
			return err
		}
		return m.Auditor.Record(ctx, AuditRecord{Action: ActionRouteReplayed, Object: audit.ObjectRoute, ObjectID: string(id),
			Team: route.TeamID, After: ReplayAuditView{ReplayID: rp.ID, From: rp.From}})
	})
	if err != nil {
		return Replay{}, err
	}
	return rp, nil
}

// Status returns one replay of a route readable by the caller.
func (m *ReplayManager) Status(ctx context.Context, id RouteID, replay ReplayID) (Replay, error) {
	if _, err := m.load(ctx, id); err != nil {
		return Replay{}, err
	}
	if m.Resetter == nil {
		return Replay{}, ErrReplayUnavailable
	}
	return m.Resetter.Status(ctx, id, replay)
}

// guard rejects routes that cannot be replayed: archived, never deployed, or no runtime.
func (m *ReplayManager) guard(r Route) error {
	switch {
	case r.DesiredState == DesiredArchived:
		return ErrArchived
	case !r.HasActiveVersion():
		return ErrNotDeployed
	case m.Resetter == nil:
		return ErrReplayUnavailable
	}
	return nil
}
