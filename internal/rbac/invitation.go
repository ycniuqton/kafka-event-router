package rbac

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/store"
)

// Invite is the command behind POST /users/invitations: a pending user with one team role.
type Invite struct {
	Email       string
	DisplayName string
	TeamID      TeamID
	Role        Role
}

// EmailAddress validates and normalises invitation emails.
type EmailAddress struct{}

// Normalise lower-cases a bare address and rejects anything else.
func (EmailAddress) Normalise(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", ErrInvalidEmail
	}
	return email, nil
}

// DisplayNameOf picks the given display name, or the email's local part.
func (EmailAddress) DisplayNameOf(email, given string) string {
	if name := strings.TrimSpace(given); name != "" {
		return name
	}
	local, _, _ := strings.Cut(email, "@")
	return local
}

// InvitationStore persists pending users and claims them at first login.
type InvitationStore interface {
	EmailExists(ctx context.Context, email string) (bool, error)
	InsertInvited(ctx context.Context, email, displayName string, by *uuid.UUID) (uuid.UUID, error)
}

// PostgresInvitationStore implements InvitationStore on users (pending = sso_subject IS NULL).
type PostgresInvitationStore struct {
	db *store.DB
}

// NewPostgresInvitationStore builds the store.
func NewPostgresInvitationStore(db *store.DB) *PostgresInvitationStore {
	return &PostgresInvitationStore{db: db}
}

// EmailExists reports whether any user (active, invited or disabled) has the email.
func (s *PostgresInvitationStore) EmailExists(ctx context.Context, email string) (bool, error) {
	var ok bool
	err := s.db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = lower($1))`, email).Scan(&ok)
	return ok, s.db.Translate(err)
}

// InsertInvited creates the pending user.
func (s *PostgresInvitationStore) InsertInvited(ctx context.Context, email, displayName string, by *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.Q(ctx).QueryRow(ctx, `INSERT INTO users (email, display_name, invited_at, invited_by) VALUES ($1, $2, now(), $3)
		RETURNING id`, email, displayName, by).Scan(&id)
	return id, s.db.Translate(err)
}

// Inviter pre-creates users with a team role before their first login (design FR-14).
type Inviter struct {
	tx         store.TxManager
	invites    InvitationStore
	bindings   BindingStore
	teams      TeamStore
	users      UserStore
	assignable RoleSet
	trail      *AuditTrail
}

// InviterDeps groups the collaborators of an Inviter.
type InviterDeps struct {
	Tx         store.TxManager
	Invites    InvitationStore
	Bindings   BindingStore
	Teams      TeamStore
	Users      UserStore
	Assignable RoleSet
	Trail      *AuditTrail
}

// NewInviter builds the inviter.
func NewInviter(d InviterDeps) *Inviter {
	return &Inviter{tx: d.Tx, invites: d.Invites, bindings: d.Bindings, teams: d.Teams, users: d.Users, assignable: d.Assignable, trail: d.Trail}
}

// Invite validates the command and creates the pending user plus binding in one transaction.
func (i *Inviter) Invite(ctx context.Context, cmd Invite) (User, error) {
	email, err := EmailAddress{}.Normalise(cmd.Email)
	if err != nil {
		return User{}, err
	}
	if !i.assignable.Has(cmd.Role) {
		return User{}, ErrRoleNotAssignable
	}
	var out User
	err = i.tx.WithinTx(ctx, func(ctx context.Context) error {
		id, err := i.create(ctx, email, cmd)
		if err != nil {
			return err
		}
		if out, err = i.users.Get(ctx, id); err != nil {
			return err
		}
		return i.trail.Record(ctx, audit.Change{Action: audit.ActionUserInvited, Object: audit.ObjectUser, ObjectID: id.String(),
			TeamID: &cmd.TeamID, After: map[string]any{"email": email, "teamId": cmd.TeamID, "role": cmd.Role}})
	})
	return out, err
}

// create checks the team and the email, then inserts the pending user and its binding.
func (i *Inviter) create(ctx context.Context, email string, cmd Invite) (uuid.UUID, error) {
	if _, err := i.teams.Get(ctx, cmd.TeamID); errors.Is(err, store.ErrNotFound) {
		return uuid.Nil, ErrUnknownTeam
	} else if err != nil {
		return uuid.Nil, err
	}
	exists, err := i.invites.EmailExists(ctx, email)
	if err != nil {
		return uuid.Nil, err
	}
	if exists {
		return uuid.Nil, ErrUserExists
	}
	id, err := i.invites.InsertInvited(ctx, email, EmailAddress{}.DisplayNameOf(email, cmd.DisplayName), PrincipalUser{}.Of(ctx))
	if err != nil {
		return uuid.Nil, err
	}
	_, err = i.bindings.Insert(ctx, id, cmd.TeamID, cmd.Role)
	return id, err
}

// PrincipalUser reads the acting user id from the request context.
type PrincipalUser struct{}

// Of returns the caller's user id, or nil for system / workload callers.
func (PrincipalUser) Of(ctx context.Context) *uuid.UUID {
	p := auth.PrincipalFrom(ctx)
	if p.UserID == uuid.Nil {
		return nil
	}
	id := p.UserID
	return &id
}

// InvitationAcceptor stamps the first login of an invited user (the users_claim_invitation trigger
// already attached the login subject to the pending row, keeping its invited bindings).
type InvitationAcceptor interface {
	Accept(ctx context.Context, userID uuid.UUID) (bool, error)
}

// Accept marks the invitation accepted; it reports false for users that were not invited or already accepted.
func (s *PostgresInvitationStore) Accept(ctx context.Context, userID uuid.UUID) (bool, error) {
	tag, err := s.db.Q(ctx).Exec(ctx, `UPDATE users SET invitation_accepted_at = now()
		WHERE id = $1 AND invited_at IS NOT NULL AND invitation_accepted_at IS NULL AND sso_subject IS NOT NULL`, userID)
	if err != nil {
		return false, s.db.Translate(err)
	}
	return tag.RowsAffected() > 0, nil
}
