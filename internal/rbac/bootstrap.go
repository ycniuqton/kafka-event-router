package rbac

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
)

// BootstrapRule decides whether a logging-in user becomes workspace admin (design flow 5.3).
type BootstrapRule interface {
	Applies(ctx context.Context, u auth.User, id auth.Identity) (bool, error)
}

// DevSeedRule promotes dev-login users (local only; the dev seeder already binds them as admin).
type DevSeedRule struct{}

// Applies implements BootstrapRule.
func (DevSeedRule) Applies(_ context.Context, _ auth.User, id auth.Identity) (bool, error) {
	return id.Mode == config.AuthModeDev, nil
}

// EmailAllowListRule promotes users whose email is in ROUTER_BOOTSTRAP_ADMIN_EMAILS (case-insensitive).
type EmailAllowListRule struct {
	emails map[string]bool
}

// NewEmailAllowListRule builds the rule.
func NewEmailAllowListRule(emails []string) EmailAllowListRule {
	set := map[string]bool{}
	for _, e := range emails {
		set[strings.ToLower(strings.TrimSpace(e))] = true
	}
	return EmailAllowListRule{emails: set}
}

// Applies implements BootstrapRule.
func (r EmailAllowListRule) Applies(_ context.Context, u auth.User, _ auth.Identity) (bool, error) {
	return r.emails[strings.ToLower(u.Email)], nil
}

// FirstAdminRule promotes the first user when the workspace has no active admin.
type FirstAdminRule struct {
	admins AdminStore
}

// NewFirstAdminRule builds the rule.
func NewFirstAdminRule(admins AdminStore) FirstAdminRule {
	return FirstAdminRule{admins: admins}
}

// Applies implements BootstrapRule.
func (r FirstAdminRule) Applies(ctx context.Context, u auth.User, _ auth.Identity) (bool, error) {
	n, err := r.admins.ActiveWorkspaceAdmins(ctx, u.ID)
	return n == 0, err
}

// TeamEnsurer creates the default team when missing.
type TeamEnsurer interface {
	EnsureByName(ctx context.Context, name string) (uuid.UUID, error)
}

// BindingEnsurer binds a role unless the user already has a binding on the team.
type BindingEnsurer interface {
	Ensure(ctx context.Context, userID, teamID uuid.UUID, role auth.Role) (bool, error)
}

// Bootstrapper is the access-control LoginHook replacing platform-core's first-user hook.
type Bootstrapper struct {
	invites  InvitationAcceptor
	rules    []BootstrapRule
	admins   AdminStore
	teams    TeamEnsurer
	bindings BindingEnsurer
	teamName string
	recorder audit.Recorder
}

// BootstrapDeps groups the collaborators of a Bootstrapper.
type BootstrapDeps struct {
	Invites  InvitationAcceptor
	Rules    []BootstrapRule
	Admins   AdminStore
	Teams    TeamEnsurer
	Bindings BindingEnsurer
	TeamName string
	Recorder audit.Recorder
}

// NewBootstrapper builds the hook.
func NewBootstrapper(d BootstrapDeps) *Bootstrapper {
	return &Bootstrapper{invites: d.Invites, rules: d.Rules, admins: d.Admins, teams: d.Teams, bindings: d.Bindings, teamName: d.TeamName, recorder: d.Recorder}
}

// OnLogin runs inside the login transaction: accept a pending invitation, then promote when a rule applies;
// otherwise the user keeps only the bindings an admin gave (or invited) them.
func (b *Bootstrapper) OnLogin(ctx context.Context, u auth.User, id auth.Identity) error {
	if err := b.admins.Lock(ctx); err != nil {
		return err
	}
	if err := b.accept(ctx, u); err != nil {
		return err
	}
	promote, err := b.anyRule(ctx, u, id)
	if err != nil || !promote {
		return err
	}
	return b.promote(ctx, u)
}

func (b *Bootstrapper) anyRule(ctx context.Context, u auth.User, id auth.Identity) (bool, error) {
	for _, r := range b.rules {
		ok, err := r.Applies(ctx, u, id)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

func (b *Bootstrapper) promote(ctx context.Context, u auth.User) error {
	changed, err := b.admins.PromoteWorkspaceAdmin(ctx, u.ID)
	if err != nil {
		return err
	}
	teamID, err := b.teams.EnsureByName(ctx, b.teamName)
	if err != nil {
		return err
	}
	if _, err := b.bindings.Ensure(ctx, u.ID, teamID, RoleAdmin); err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return b.recorder.Record(ctx, audit.Entry{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: audit.ActionUserBootstrapAdmin,
		Object: audit.ObjectUser, ObjectID: u.ID.String(), ObjectLabel: u.Email})
}

// accept records the first login of an invited user (no acceptor: nothing to do).
func (b *Bootstrapper) accept(ctx context.Context, u auth.User) error {
	if b.invites == nil {
		return nil
	}
	accepted, err := b.invites.Accept(ctx, u.ID)
	if err != nil || !accepted {
		return err
	}
	return b.recorder.Record(ctx, audit.Entry{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: audit.ActionInvitationAccepted,
		Object: audit.ObjectUser, ObjectID: u.ID.String(), ObjectLabel: u.Email})
}
