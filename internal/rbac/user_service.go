package rbac

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/store"
)

// DisplayNameLimit bounds display names.
type DisplayNameLimit int

const MaxDisplayName DisplayNameLimit = 200

// SessionRevoker ends every session of a user (platform-core session store).
type SessionRevoker interface {
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
}

// UserService lists and administers users (display name, workspace admin, disable) with the last-admin guard.
type UserService struct {
	tx       store.TxManager
	users    UserStore
	guard    *AdminGuard
	sessions SessionRevoker
	trail    *AuditTrail
}

// UserServiceDeps groups the collaborators of a UserService.
type UserServiceDeps struct {
	Tx       store.TxManager
	Users    UserStore
	Guard    *AdminGuard
	Sessions SessionRevoker
	Trail    *AuditTrail
}

// NewUserService builds the service.
func NewUserService(d UserServiceDeps) *UserService {
	return &UserService{tx: d.Tx, users: d.Users, guard: d.Guard, sessions: d.Sessions, trail: d.Trail}
}

// List returns one page of users.
func (s *UserService) List(ctx context.Context, f UserFilter, page Page) ([]User, *store.Cursor, error) {
	return s.users.List(ctx, f, page)
}

// Get returns one user.
func (s *UserService) Get(ctx context.Context, id uuid.UUID) (User, error) {
	return s.users.Get(ctx, id)
}

// Update applies c under If-Match; disabling a user also revokes their sessions after commit.
func (s *UserService) Update(ctx context.Context, id uuid.UUID, c UserChange, ifMatch store.RowVersion) (User, error) {
	c, err := s.normalise(c)
	if err != nil {
		return User{}, err
	}
	var after User
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		after, err = s.apply(ctx, id, c, ifMatch)
		return err
	})
	if err != nil {
		return User{}, err
	}
	if c.Disabled != nil && *c.Disabled {
		return after, s.sessions.RevokeAllForUser(ctx, id)
	}
	return after, nil
}

func (s *UserService) apply(ctx context.Context, id uuid.UUID, c UserChange, ifMatch store.RowVersion) (User, error) {
	if err := s.guard.Lock(ctx); err != nil {
		return User{}, err
	}
	before, err := s.users.Get(ctx, id)
	if err != nil {
		return User{}, err
	}
	if ifMatch != before.RowVersion {
		return User{}, &store.PreconditionFailedError{Current: before.RowVersion}
	}
	if err := s.guard.CheckUserLoss(ctx, before, c); err != nil {
		return User{}, err
	}
	if err := s.users.Update(ctx, id, c, ifMatch); err != nil {
		return User{}, err
	}
	after, err := s.users.Get(ctx, id)
	if err != nil {
		return User{}, err
	}
	return after, s.trail.Record(ctx, audit.Change{Action: audit.ActionUserUpdated, Object: audit.ObjectUser, ObjectID: id.String(),
		Before: UserAudit{}.Of(before), After: UserAudit{}.Of(after)})
}

func (s *UserService) normalise(c UserChange) (UserChange, error) {
	if c.DisplayName == nil {
		return c, nil
	}
	name := strings.TrimSpace(*c.DisplayName)
	if name == "" || utf8.RuneCountInString(name) > int(MaxDisplayName) {
		return c, ErrInvalidDisplayName
	}
	c.DisplayName = &name
	return c, nil
}

// UserAudit renders users for audit diffs.
type UserAudit struct{}

// Of returns the audited fields of u.
func (UserAudit) Of(u User) map[string]any {
	return map[string]any{"displayName": u.DisplayName, "workspaceAdmin": u.WorkspaceAdmin, "status": u.Status}
}
