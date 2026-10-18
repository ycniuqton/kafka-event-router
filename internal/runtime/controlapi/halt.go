package controlapi

import (
	"context"
	"log/slog"
	"time"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/state"
)

// HaltService applies a worker's halt report (design 5.3): a halt of a version still rolling out is a
// failed rollout (auto-rollback); a halt of a version that is no longer active (late replica of a rolled
// back version) is dropped; any other halt pauses the route and records the halt until resume or deploy.
type HaltService struct {
	Stores    state.Stores
	Lifecycle state.DeploymentLifecycle
	Pauser    state.RoutePauser
	Log       *slog.Logger
	Now       func() time.Time
}

// Halt implements the halt endpoint.
func (s *HaltService) Halt(ctx context.Context, h domain.HaltReport) error {
	rolledBack, err := s.failRollout(ctx, h)
	if err != nil || rolledBack {
		return err
	}
	if stale, err := s.superseded(ctx, h); err != nil || stale {
		return err
	}
	if err := s.Pauser.Pause(ctx, h.RouteID); err != nil {
		s.Log.Error("runtime: pausing halted route failed", string(logging.AttrRouteID), h.RouteID, string(logging.AttrError), err)
	}
	return s.Stores.Runtime.Halt(ctx, h, s.Now())
}

// failRollout marks the rolling deployment of the halted version failed; true when routes rolled back.
func (s *HaltService) failRollout(ctx context.Context, h domain.HaltReport) (bool, error) {
	rolling, err := s.Stores.Rollouts.Rolling(ctx)
	if err != nil {
		return false, err
	}
	for _, r := range rolling {
		if r.RouteID != h.RouteID || r.VersionID != h.VersionID {
			continue
		}
		paused, err := s.Lifecycle.MarkFailed(ctx, r.DeploymentID, h.Message)
		if err != nil || !paused {
			return err == nil, err
		}
		h.Reason = domain.HaltAutoRollbackFailed
		return true, s.Stores.Runtime.Halt(ctx, h, s.Now())
	}
	return false, nil
}

// superseded reports whether the halted version was already replaced as the route's active version.
func (s *HaltService) superseded(ctx context.Context, h domain.HaltReport) (bool, error) {
	if h.VersionID == "" {
		return false, nil
	}
	active, err := s.Stores.Routes.ActiveVersion(ctx, h.RouteID)
	if err != nil || active == h.VersionID {
		return false, err
	}
	s.Log.Info("runtime: halt of a superseded version ignored", string(logging.AttrRouteID), h.RouteID, string(logging.AttrVersion), h.VersionID)
	return true, nil
}
