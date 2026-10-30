package rbac

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
)

// ChannelRBACChanged is the NOTIFY channel fed by the 0005 triggers on users and role_bindings.
const ChannelRBACChanged store.Channel = "rbac_changed"

// ChangeNotice is the payload of rbac_changed.
type ChangeNotice struct {
	UserID uuid.UUID `json:"user_id"`
}

// InvalidationListener drops changed subjects from the cache (all API instances). It subscribes to rbac_changed
// on the process listener hub (dedicated session, reconnect and backoff live there).
type InvalidationListener struct {
	source store.NotificationSource
	cache  SubjectCache
	log    *slog.Logger
}

// NewInvalidationListener builds the listener.
func NewInvalidationListener(source store.NotificationSource, cache SubjectCache, log *slog.Logger) *InvalidationListener {
	return &InvalidationListener{source: source, cache: cache, log: log}
}

// Run applies notices until ctx ends; a hub reset (connect, reconnect, overflow) invalidates everyone.
func (l *InvalidationListener) Run(ctx context.Context) {
	sub := l.source.Subscribe(ChannelRBACChanged)
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case n, ok := <-sub.C():
			if !ok {
				return
			}
			l.handle(n)
		}
	}
}

// handle applies one hub notice.
func (l *InvalidationListener) handle(n store.Notice) {
	if n.Kind == store.NoticeReset {
		l.cache.InvalidateAll()
		return
	}
	l.Apply(n.Payload)
}

// Apply invalidates the user named by payload, or everyone when it cannot be parsed.
func (l *InvalidationListener) Apply(payload string) {
	var notice ChangeNotice
	if err := json.Unmarshal([]byte(payload), &notice); err != nil || notice.UserID == uuid.Nil {
		l.cache.InvalidateAll()
		return
	}
	l.cache.Invalidate(notice.UserID)
}
