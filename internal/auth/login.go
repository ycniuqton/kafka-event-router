package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/problem"
)

// Identity is what an interactive login flow proves about the user.
type Identity struct {
	Mode        AuthMode
	Subject     string
	Email       string
	DisplayName string
	ReturnTo    string
	Groups      []string
}

// User is the persisted user an identity maps to; Disabled users cannot authenticate.
type User struct {
	ID          uuid.UUID
	Email       string
	DisplayName string
	Disabled    bool
}

// LoginFlow is the base of every interactive login (OIDC, dev).
type LoginFlow interface {
	Mode() AuthMode
	Begin(w http.ResponseWriter, r *http.Request) error
	Complete(w http.ResponseWriter, r *http.Request) (Identity, error)
}

// UserProvisioner creates or updates the user row for an identity (by sso_subject).
type UserProvisioner interface {
	Upsert(ctx context.Context, id Identity) (User, error)
}

// LoginHook runs after the user row exists (first-user admin here; access-control's Bootstrapper replaces it).
type LoginHook interface {
	OnLogin(ctx context.Context, u User, id Identity) error
}

// LoginTracker records when a user last signed in (users.last_login_at).
type LoginTracker interface {
	MarkLogin(ctx context.Context, userID uuid.UUID, at time.Time) error
}

// NoLoginTracking is the LoginTracker used when none is configured.
type NoLoginTracking struct{}

// MarkLogin implements LoginTracker.
func (NoLoginTracking) MarkLogin(context.Context, uuid.UUID, time.Time) error { return nil }

// ErrUserDisabled rejects logins and sessions of users a workspace admin disabled.
var ErrUserDisabled = problem.NewError(problem.KindForbidden, "Your account is disabled. Ask a workspace admin to enable it.")

// ErrLoginModeDisabled is returned when a login flow is not configured.
var ErrLoginModeDisabled = problem.NewError(problem.KindNotFound, "This sign-in method is not enabled on this server.")
