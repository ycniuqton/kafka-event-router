package events

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/store"
)

// RouteStatePayload is the payload of route.state_changed.
type RouteStatePayload struct {
	RouteID string        `json:"routeId"`
	Status  routes.Status `json:"status"`
	Error   string        `json:"error,omitempty"`
}

// DeploymentPayload is the payload of deployment.status_changed.
type DeploymentPayload struct {
	RouteID      string `json:"routeId"`
	DeploymentID string `json:"deploymentId"`
	Status       string `json:"status"`
}

// RouteSnapshot is the current state of a route as push events report it.
type RouteSnapshot struct {
	TeamID       string
	Status       routes.Status
	Error        string
	DeploymentID string
	Deployment   string
}

// RouteStateReader reads a route's status and latest deployment.
type RouteStateReader interface {
	Snapshot(ctx context.Context, routeID string) (RouteSnapshot, bool, error)
}

const routeSnapshotSQL = `SELECT r.team_id::text, r.desired_state::text, r.active_version_id IS NOT NULL,
       COALESCE(rt.observed_state::text, ''), COALESCE(rt.error_message, ''),
       COALESCE(d.id::text, ''), COALESCE(d.status::text, '')
  FROM routes r LEFT JOIN route_runtime rt ON rt.route_id = r.id
  LEFT JOIN LATERAL (SELECT id, status FROM deployments WHERE route_id = r.id ORDER BY requested_at DESC, id DESC LIMIT 1) d ON true
 WHERE r.id = $1`

// PgRouteStateReader implements RouteStateReader with routes' status resolution.
type PgRouteStateReader struct {
	DB *store.DB
}

// Snapshot returns false for unknown (deleted) routes.
func (r PgRouteStateReader) Snapshot(ctx context.Context, routeID string) (RouteSnapshot, bool, error) {
	var s RouteSnapshot
	var desired, observed string
	var active bool
	err := r.DB.Q(ctx).QueryRow(ctx, routeSnapshotSQL, routeID).Scan(&s.TeamID, &desired, &active, &observed, &s.Error,
		&s.DeploymentID, &s.Deployment)
	if errors.Is(err, pgx.ErrNoRows) {
		return RouteSnapshot{}, false, nil
	}
	if err != nil {
		return RouteSnapshot{}, false, err
	}
	s.Status = routes.RouteStatusResolver{}.Resolve(routes.DesiredState(desired), active, routes.ObservedState(observed))
	return s, true, nil
}

// HintSource yields router_runtime change hints (route-runtime's changes.Broker fed by its PgListener).
type HintSource interface {
	Subscribe() (<-chan domain.ChangeHint, func())
}

// RuntimeChangeSource turns route-runtime NOTIFY hints into route.state_changed and deployment.status_changed.
// Every API instance LISTENs itself, so it publishes to its local hub only; unchanged states are not re-sent.
type RuntimeChangeSource struct {
	Hints   HintSource
	Reader  RouteStateReader
	Out     Publisher
	Factory EventFactory
	Log     *slog.Logger

	mu   sync.Mutex
	last map[string]RouteSnapshot
}

// Run relays hints until ctx ends.
func (s *RuntimeChangeSource) Run(ctx context.Context) {
	hints, cancel := s.Hints.Subscribe()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case h, ok := <-hints:
			if !ok {
				return
			}
			if err := s.Handle(ctx, h); err != nil {
				s.Log.Warn("events: runtime change relay failed", string(logging.AttrError), err)
			}
		}
	}
}

// Handle publishes the changes one hint reveals.
func (s *RuntimeChangeSource) Handle(ctx context.Context, h domain.ChangeHint) error {
	if h.RouteID == "" {
		return nil
	}
	snap, ok, err := s.Reader.Snapshot(ctx, h.RouteID)
	if err != nil || !ok {
		return err
	}
	prev, seen := s.swap(h.RouteID, snap)
	if !seen || prev.Status != snap.Status || prev.Error != snap.Error {
		if err := s.emit(ctx, TypeRouteStateChanged, snap.TeamID, RouteStatePayload{RouteID: h.RouteID, Status: snap.Status, Error: snap.Error}); err != nil {
			return err
		}
	}
	if snap.DeploymentID != "" && (!seen || prev.DeploymentID != snap.DeploymentID || prev.Deployment != snap.Deployment) {
		return s.emit(ctx, TypeDeploymentStatus, snap.TeamID, DeploymentPayload{RouteID: h.RouteID, DeploymentID: snap.DeploymentID, Status: snap.Deployment})
	}
	return nil
}

func (s *RuntimeChangeSource) swap(id string, snap RouteSnapshot) (RouteSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		s.last = map[string]RouteSnapshot{}
	}
	prev, ok := s.last[id]
	s.last[id] = snap
	return prev, ok
}

func (s *RuntimeChangeSource) emit(ctx context.Context, t PushEventType, team string, payload any) error {
	e, err := s.Factory.New(t, team, payload)
	if err != nil {
		return err
	}
	return s.Out.Publish(ctx, e)
}
