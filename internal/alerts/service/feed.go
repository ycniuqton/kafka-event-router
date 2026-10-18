package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/alerts"
	"github.com/eventrouter/router/internal/store"
)

// FeedLimit bounds GET /alerts pages.
type FeedLimit int

const DefaultFeedLimit FeedLimit = 20

// FeedService serves the bell: alert history with the caller's unread count, and acknowledgements.
type FeedService struct {
	Events alerts.EventRepository
	Now    func() time.Time
}

// List returns one page for user; state filters firing or resolved.
func (s *FeedService) List(ctx context.Context, user uuid.UUID, state *alerts.AlertState, limit int, after *store.Cursor) (alerts.FeedPage, error) {
	if state != nil && *state != alerts.StateFiring && *state != alerts.StateResolved {
		return alerts.FeedPage{}, alerts.ErrInvalidState.At(string(alerts.PathState))
	}
	return s.Events.Feed(ctx, alerts.FeedQuery{State: state, User: user, Limit: limit, After: after})
}

// Ack records that user acknowledged the alert (idempotent).
func (s *FeedService) Ack(ctx context.Context, id alerts.EventID, user uuid.UUID) error {
	return s.Events.Ack(ctx, id, user, s.Now())
}

// Firing returns every firing alert (needs-attention rows).
func (s *FeedService) Firing(ctx context.Context) ([]alerts.FeedItem, error) {
	return s.Events.FiringItems(ctx)
}
