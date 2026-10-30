package password

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/internal/store/identity"
)

// SeedAccounts finds or creates the first-start admin's user row and makes it workspace admin.
type SeedAccounts interface {
	Lock(ctx context.Context) error
	FindOrCreate(ctx context.Context, email, displayName string) (uuid.UUID, error)
	PromoteWorkspaceAdmin(ctx context.Context, userID uuid.UUID) error
}

// TeamEnsurer creates the default team when missing.
type TeamEnsurer interface {
	EnsureByName(ctx context.Context, name string) (uuid.UUID, error)
}

// BindingEnsurer creates a role binding when missing.
type BindingEnsurer interface {
	Ensure(ctx context.Context, userID, teamID uuid.UUID, role auth.Role) (bool, error)
}

// SeedLog names the log lines of the first-start admin seed.
type SeedLog string

const (
	LogSeedSkipped SeedLog = "password login: no password user exists and ROUTER_BOOTSTRAP_ADMIN_EMAIL is unset; nobody can sign in with a password yet"
	LogSeedCreated SeedLog = "password login: bootstrap admin created; it must change its password at first login"
	LogAttrEmail   SeedLog = "email"
)

// AdminSeeder creates the first admin from env on first start (no password user yet); it never runs again.
type AdminSeeder struct {
	Creds    *Credentials
	Accounts SeedAccounts
	Teams    TeamEnsurer
	Bindings BindingEnsurer
	Tx       store.TxManager
	Recorder audit.Recorder
	Config   config.PasswordLoginConfig
	TeamName string
	Log      *slog.Logger
}

// Run seeds the admin inside one transaction under the bootstrap advisory lock (safe with several replicas).
func (s *AdminSeeder) Run(ctx context.Context) error {
	email := strings.ToLower(strings.TrimSpace(s.Config.BootstrapEmail))
	return s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.Accounts.Lock(ctx); err != nil {
			return err
		}
		seeded, err := s.Creds.Seeded(ctx)
		if err != nil || seeded {
			return err
		}
		if email == "" {
			s.Log.Warn(string(LogSeedSkipped))
			return nil
		}
		return s.seed(ctx, email)
	})
}

func (s *AdminSeeder) seed(ctx context.Context, email string) error {
	plain, err := BootstrapSecret{Config: s.Config}.Read()
	if err != nil {
		return err
	}
	if err := s.Creds.Policy().Check(plain); err != nil {
		return fmt.Errorf("password: %s: %w", config.EnvBootstrapAdminPassword, err)
	}
	local, _, _ := strings.Cut(email, "@")
	userID, err := s.Accounts.FindOrCreate(ctx, email, local)
	if err != nil {
		return err
	}
	if _, err := s.Creds.Assign(ctx, userID, plain, true); err != nil {
		return err
	}
	if err := s.promote(ctx, userID); err != nil {
		return err
	}
	s.Log.Info(string(LogSeedCreated), string(LogAttrEmail), email)
	return s.Recorder.Record(ctx, audit.Entry{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: audit.ActionUserBootstrapAdmin,
		Object: audit.ObjectUser, ObjectID: userID.String(), ObjectLabel: email})
}

func (s *AdminSeeder) promote(ctx context.Context, userID uuid.UUID) error {
	if err := s.Accounts.PromoteWorkspaceAdmin(ctx, userID); err != nil {
		return err
	}
	teamID, err := s.Teams.EnsureByName(ctx, s.TeamName)
	if err != nil {
		return err
	}
	_, err = s.Bindings.Ensure(ctx, userID, teamID, auth.RoleAdmin)
	return err
}

// BootstrapSecret reads the bootstrap admin password from env or from its file.
type BootstrapSecret struct {
	Config config.PasswordLoginConfig
}

// ErrNoBootstrapPassword means the bootstrap email is set without a password.
var ErrNoBootstrapPassword = errors.New("password: ROUTER_BOOTSTRAP_ADMIN_PASSWORD or ROUTER_BOOTSTRAP_ADMIN_PASSWORD_FILE is required with ROUTER_BOOTSTRAP_ADMIN_EMAIL")

// Read returns the password; the file wins over the variable and loses one trailing newline.
func (b BootstrapSecret) Read() (string, error) {
	if b.Config.BootstrapPasswordFile == "" {
		if b.Config.BootstrapPassword == "" {
			return "", ErrNoBootstrapPassword
		}
		return b.Config.BootstrapPassword, nil
	}
	raw, err := os.ReadFile(b.Config.BootstrapPasswordFile)
	if err != nil {
		return "", fmt.Errorf("password: read %s: %w", config.EnvBootstrapAdminPasswordFile, err)
	}
	return strings.TrimRight(string(raw), "\r\n"), nil
}

// PostgresSeedAccounts implements SeedAccounts on the users table.
type PostgresSeedAccounts struct {
	db *store.DB
}

// NewPostgresSeedAccounts builds the store.
func NewPostgresSeedAccounts(db *store.DB) *PostgresSeedAccounts {
	return &PostgresSeedAccounts{db: db}
}

// Lock takes the identity bootstrap advisory lock for the transaction.
func (s *PostgresSeedAccounts) Lock(ctx context.Context) error {
	_, err := s.db.Q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(identity.BootstrapAdminLock))
	return s.db.Translate(err)
}

// FindOrCreate returns the user with email, inserting a password user when none exists.
func (s *PostgresSeedAccounts) FindOrCreate(ctx context.Context, email, displayName string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.Q(ctx).QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1)`, email).Scan(&id)
	if err = s.db.Translate(err); !errors.Is(err, store.ErrNotFound) {
		return id, err
	}
	id = uuid.New()
	_, err = s.db.Q(ctx).Exec(ctx, `INSERT INTO users (id, email, display_name, sso_subject) VALUES ($1, $2, $3, $4)`,
		id, email, displayName, string(PasswordSubjectPrefix)+id.String())
	return id, s.db.Translate(err)
}

// PromoteWorkspaceAdmin sets the workspace-admin flag.
func (s *PostgresSeedAccounts) PromoteWorkspaceAdmin(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Q(ctx).Exec(ctx, `UPDATE users SET is_workspace_admin = true WHERE id = $1 AND NOT is_workspace_admin`, userID)
	return s.db.Translate(err)
}
