// Package dev is the local-only dev login: it seeds a default team and an admin user and issues
// a normal session. It is registered only when ROUTER_ENV=local and "dev" is in ROUTER_AUTH_MODES.
package dev

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/store"
)

// SubjectPrefix marks sso_subject values of dev users.
type SubjectPrefix string

const DevSubjectPrefix SubjectPrefix = "dev:"

// EmailMarker is the character every email address must contain.
type EmailMarker string

const EmailAt EmailMarker = "@"

// LoginRequest is the optional body of POST /auth/dev/login.
type LoginRequest struct {
	Email string `json:"email"`
}

// TeamEnsurer creates the default team when missing.
type TeamEnsurer interface {
	EnsureByName(ctx context.Context, name string) (uuid.UUID, error)
}

// BindingEnsurer creates a role binding when missing.
type BindingEnsurer interface {
	Ensure(ctx context.Context, userID, teamID uuid.UUID, role auth.Role) (bool, error)
}

// Seeder ensures the dev team, user and admin binding exist (idempotent).
type Seeder struct {
	Users    auth.UserProvisioner
	Teams    TeamEnsurer
	Bindings BindingEnsurer
	Tx       store.TxManager
	Recorder audit.Recorder
	Config   config.DevLoginConfig
}

// Ensure seeds the rows for email and records dev.seed when anything was created.
func (s *Seeder) Ensure(ctx context.Context, id auth.Identity) error {
	return s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		teamID, err := s.Teams.EnsureByName(ctx, s.Config.TeamName)
		if err != nil {
			return err
		}
		user, err := s.Users.Upsert(ctx, id)
		if err != nil {
			return err
		}
		created, err := s.Bindings.Ensure(ctx, user.ID, teamID, auth.RoleAdmin)
		if err != nil || !created {
			return err
		}
		return s.Recorder.Record(ctx, audit.Entry{Actor: audit.Actor{Kind: audit.ActorSystem}, Action: audit.ActionDevSeed,
			Object: audit.ObjectUser, ObjectID: user.ID.String()})
	})
}

// Flow is the dev LoginFlow.
type Flow struct {
	seeder *Seeder
	cfg    config.DevLoginConfig
}

// NewFlow builds the dev flow.
func NewFlow(seeder *Seeder, cfg config.DevLoginConfig) *Flow {
	return &Flow{seeder: seeder, cfg: cfg}
}

// Mode implements auth.LoginFlow.
func (f *Flow) Mode() auth.AuthMode { return config.AuthModeDev }

// Begin is not used by the dev flow (single POST).
func (f *Flow) Begin(http.ResponseWriter, *http.Request) error { return auth.ErrLoginModeDisabled }

// Complete reads the optional email, seeds the rows and returns the dev identity.
func (f *Flow) Complete(w http.ResponseWriter, r *http.Request) (auth.Identity, error) {
	req, err := api.Decoder[LoginRequest]{AllowEmpty: true}.Decode(w, r)
	if err != nil {
		return auth.Identity{}, err
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		email = f.cfg.AdminEmail
	}
	if !strings.Contains(email, string(EmailAt)) {
		return auth.Identity{}, api.NewValidationError().Add("/email", "email", "Enter an email address such as admin@local.dev.")
	}
	id := auth.Identity{Mode: config.AuthModeDev, Subject: string(DevSubjectPrefix) + email, Email: email,
		DisplayName: strings.Split(email, string(EmailAt))[0]}
	return id, f.seeder.Ensure(r.Context(), id)
}
