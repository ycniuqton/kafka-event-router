package rbac

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth/password"
	"github.com/eventrouter/router/internal/store"
)

// NewPasswordUser is the command behind POST /users: an active password user, optionally with one team role.
type NewPasswordUser struct {
	Email          string
	DisplayName    string
	Password       string
	WorkspaceAdmin bool
	TeamID         *TeamID
	Role           Role
}

// PasswordAccountStore inserts users that sign in with a password.
type PasswordAccountStore interface {
	EmailExists(ctx context.Context, email string) (bool, error)
	InsertPasswordUser(ctx context.Context, email, displayName string, workspaceAdmin bool) (uuid.UUID, error)
}

// PasswordUserService lets workspace admins create password users and reset passwords (no self-registration).
type PasswordUserService struct {
	tx         store.TxManager
	accounts   PasswordAccountStore
	creds      *password.Credentials
	users      UserStore
	bindings   BindingStore
	teams      TeamStore
	assignable RoleSet
	sessions   SessionRevoker
	trail      *AuditTrail
}

// PasswordUserDeps groups the collaborators of a PasswordUserService.
type PasswordUserDeps struct {
	Tx         store.TxManager
	Accounts   PasswordAccountStore
	Creds      *password.Credentials
	Users      UserStore
	Bindings   BindingStore
	Teams      TeamStore
	Assignable RoleSet
	Sessions   SessionRevoker
	Trail      *AuditTrail
}

// NewPasswordUserService builds the service.
func NewPasswordUserService(d PasswordUserDeps) *PasswordUserService {
	return &PasswordUserService{tx: d.Tx, accounts: d.Accounts, creds: d.Creds, users: d.Users, bindings: d.Bindings,
		teams: d.Teams, assignable: d.Assignable, sessions: d.Sessions, trail: d.Trail}
}

// Create validates cmd and creates the user, its password (forced change) and its binding in one transaction.
func (s *PasswordUserService) Create(ctx context.Context, cmd NewPasswordUser) (User, password.Assignment, error) {
	cmd, err := s.normalise(cmd)
	if err != nil {
		return User{}, password.Assignment{}, err
	}
	var out User
	var assigned password.Assignment
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		id, err := s.insert(ctx, cmd)
		if err != nil {
			return err
		}
		if assigned, err = s.creds.Assign(ctx, id, cmd.Password, true); err != nil {
			return err
		}
		if out, err = s.bind(ctx, id, cmd); err != nil {
			return err
		}
		return s.trail.Record(ctx, audit.Change{Action: audit.ActionUserCreated, Object: audit.ObjectUser, ObjectID: id.String(),
			TeamID: cmd.TeamID, After: PasswordUserAudit{}.Of(out, cmd)})
	})
	return out, assigned, err
}

// Reset sets a new (given or generated) password with a forced change and ends the user's sessions.
func (s *PasswordUserService) Reset(ctx context.Context, id uuid.UUID, plain string) (password.Assignment, error) {
	var assigned password.Assignment
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.users.Get(ctx, id); err != nil {
			return err
		}
		var err error
		if assigned, err = s.creds.Assign(ctx, id, plain, true); err != nil {
			return err
		}
		return s.trail.Record(ctx, audit.Change{Action: audit.ActionUserPasswordReset, Object: audit.ObjectUser, ObjectID: id.String()})
	})
	if err != nil {
		return password.Assignment{}, err
	}
	return assigned, s.sessions.RevokeAllForUser(ctx, id)
}

func (s *PasswordUserService) normalise(cmd NewPasswordUser) (NewPasswordUser, error) {
	email, err := EmailAddress{}.Normalise(cmd.Email)
	if err != nil {
		return cmd, err
	}
	cmd.Email = email
	cmd.DisplayName = strings.TrimSpace(EmailAddress{}.DisplayNameOf(email, cmd.DisplayName))
	if utf8.RuneCountInString(cmd.DisplayName) > int(MaxDisplayName) {
		return cmd, ErrInvalidDisplayName
	}
	if cmd.TeamID != nil && !s.assignable.Has(cmd.Role) {
		return cmd, ErrRoleNotAssignable
	}
	if cmd.Password != "" && s.creds.Policy().Check(cmd.Password) != nil {
		return cmd, password.ErrWeakPassword.At(password.PathPassword)
	}
	return cmd, nil
}

// insert checks the team and the email, then inserts the user row.
func (s *PasswordUserService) insert(ctx context.Context, cmd NewPasswordUser) (uuid.UUID, error) {
	if cmd.TeamID != nil {
		if _, err := s.teams.Get(ctx, *cmd.TeamID); errors.Is(err, store.ErrNotFound) {
			return uuid.Nil, ErrUnknownTeam
		} else if err != nil {
			return uuid.Nil, err
		}
	}
	exists, err := s.accounts.EmailExists(ctx, cmd.Email)
	if err != nil {
		return uuid.Nil, err
	}
	if exists {
		return uuid.Nil, ErrUserExists
	}
	return s.accounts.InsertPasswordUser(ctx, cmd.Email, cmd.DisplayName, cmd.WorkspaceAdmin)
}

// bind adds the optional team role and returns the created user.
func (s *PasswordUserService) bind(ctx context.Context, id uuid.UUID, cmd NewPasswordUser) (User, error) {
	if cmd.TeamID != nil {
		if _, err := s.bindings.Insert(ctx, id, *cmd.TeamID, cmd.Role); err != nil {
			return User{}, err
		}
	}
	return s.users.Get(ctx, id)
}

// PasswordUserAudit renders created password users for audit diffs (never the password).
type PasswordUserAudit struct{}

// Of returns the audited fields of a created user.
func (PasswordUserAudit) Of(u User, cmd NewPasswordUser) map[string]any {
	return map[string]any{"email": u.Email, "displayName": u.DisplayName, "workspaceAdmin": u.WorkspaceAdmin,
		"teamId": cmd.TeamID, "role": cmd.Role, "passwordGenerated": cmd.Password == ""}
}

// PostgresPasswordAccountStore implements PasswordAccountStore on users.
type PostgresPasswordAccountStore struct {
	*PostgresInvitationStore
}

// NewPostgresPasswordAccountStore builds the store.
func NewPostgresPasswordAccountStore(db *store.DB) *PostgresPasswordAccountStore {
	return &PostgresPasswordAccountStore{PostgresInvitationStore: NewPostgresInvitationStore(db)}
}

// InsertPasswordUser creates an active user with the password login subject.
func (s *PostgresPasswordAccountStore) InsertPasswordUser(ctx context.Context, email, displayName string, workspaceAdmin bool) (uuid.UUID, error) {
	id := uuid.New()
	_, err := s.db.Q(ctx).Exec(ctx, `INSERT INTO users (id, email, display_name, sso_subject, is_workspace_admin) VALUES ($1, $2, $3, $4, $5)`,
		id, email, displayName, string(password.PasswordSubjectPrefix)+id.String(), workspaceAdmin)
	return id, s.db.Translate(err)
}
