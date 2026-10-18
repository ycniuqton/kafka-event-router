package auth

import (
	"context"

	"github.com/eventrouter/router/internal/audit"
)

// AdminBootstrapper binds the first user ever as admin of the default team (atomic, race-safe).
type AdminBootstrapper interface {
	BindFirstAdmin(ctx context.Context, u User, teamName string) (bool, error)
}

// FirstUserAdminHook is the default LoginHook: first login → admin on the default team; later users get nothing.
type FirstUserAdminHook struct {
	bootstrapper AdminBootstrapper
	teamName     string
	recorder     audit.Recorder
}

// NewFirstUserAdminHook builds the default hook.
func NewFirstUserAdminHook(b AdminBootstrapper, teamName string, recorder audit.Recorder) *FirstUserAdminHook {
	return &FirstUserAdminHook{bootstrapper: b, teamName: teamName, recorder: recorder}
}

// OnLogin binds the first user as admin and records user.bootstrap_admin in the same transaction.
func (h *FirstUserAdminHook) OnLogin(ctx context.Context, u User, _ Identity) error {
	bound, err := h.bootstrapper.BindFirstAdmin(ctx, u, h.teamName)
	if err != nil || !bound {
		return err
	}
	return h.recorder.Record(ctx, audit.Entry{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: audit.ActionUserBootstrapAdmin,
		Object: audit.ObjectUser, ObjectID: u.ID.String()})
}
