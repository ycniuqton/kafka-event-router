package rbac

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/store"
)

// ChannelRBACChanged is the NOTIFY channel fed by the 0005 triggers on users and role_bindings.
const ChannelRBACChanged store.Channel = "rbac_changed"

// ReconnectBackoff bounds the listener's retry delay.
type ReconnectBackoff time.Duration

const (
	MinReconnectBackoff ReconnectBackoff = ReconnectBackoff(500 * time.Millisecond)
	MaxReconnectBackoff ReconnectBackoff = ReconnectBackoff(30 * time.Second)
)

// ChangeNotice is the payload of rbac_changed.
type ChangeNotice struct {
	UserID uuid.UUID `json:"user_id"`
}

// InvalidationListener LISTENs on rbac_changed and drops changed subjects from the cache (all API instances).
type InvalidationListener struct {
	pool  *pgxpool.Pool
	cache SubjectCache
	log   *slog.Logger
}

// NewInvalidationListener builds the listener.
func NewInvalidationListener(pool *pgxpool.Pool, cache SubjectCache, log *slog.Logger) *InvalidationListener {
	return &InvalidationListener{pool: pool, cache: cache, log: log}
}

// Run listens until ctx ends, reconnecting with exponential backoff.
func (l *InvalidationListener) Run(ctx context.Context) {
	backoff := time.Duration(MinReconnectBackoff)
	for ctx.Err() == nil {
		err := l.listen(ctx)
		if ctx.Err() != nil {
			return
		}
		l.log.Warn("rbac invalidation listener reconnecting", string(logging.AttrError), err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Duration(MaxReconnectBackoff))
	}
}

func (l *InvalidationListener) listen(ctx context.Context) error {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN "+string(ChannelRBACChanged)); err != nil {
		return err
	}
	l.cache.InvalidateAll()
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		l.Apply(n.Payload)
	}
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
