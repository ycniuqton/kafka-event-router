package auth

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/problem"
)

// Session is one interactive login, stored only in Redis.
type Session struct {
	ID         string
	UserID     uuid.UUID
	Mode       AuthMode
	CSRFToken  string
	IP         string
	UserAgent  string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

// SessionStore persists sessions with idle and absolute TTLs.
type SessionStore interface {
	Create(ctx context.Context, s Session) (Session, error)
	Get(ctx context.Context, id string) (Session, error)
	Touch(ctx context.Context, s Session) error
	Revoke(ctx context.Context, id string) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
}

// PrincipalSource loads a user and their team roles to build a Principal.
type PrincipalSource interface {
	LoadUser(ctx context.Context, id uuid.UUID) (User, []TeamRole, error)
}

// ErrSessionNotFound is returned for missing or expired sessions.
var ErrSessionNotFound = problem.NewError(problem.KindUnauthenticated, "Your session expired. Sign in again.")
