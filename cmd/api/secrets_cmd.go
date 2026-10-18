package main

import (
	"context"
	"fmt"
	"io"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/app"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/internal/secrets/kms"
	"github.com/eventrouter/router/internal/secrets/local"
	"github.com/eventrouter/router/internal/secrets/vault"
	"github.com/eventrouter/router/internal/store"
)

// SecretsParts are the secret-storage components shared by the API and the `api secrets` CLI.
type SecretsParts struct {
	Provider secrets.Provider
	Repo     *secrets.PostgresRepository
	Manager  *secrets.EnvelopeManager
}

// Rotator builds the master-key rotator acting as actor.
func (p SecretsParts) Rotator(recorder audit.Recorder, actor audit.Actor, cfg config.SecretsRotationConfig) *secrets.Rotator {
	return secrets.NewRotator(secrets.RotatorDeps{Provider: p.Provider, Repo: p.Repo, Recorder: recorder, Actor: actor, PageSize: cfg.PageSize})
}

// SecretsAssembly builds the configured master-key provider (registry-selected) and the envelope manager.
type SecretsAssembly struct{}

// Build composes the parts over db.
func (SecretsAssembly) Build(ctx context.Context, cfg config.SecretsConfig, db *store.DB) (SecretsParts, error) {
	reg := secrets.NewRegistry()
	local.Register(reg)
	vault.Register(reg)
	kms.Register(reg)
	provider, err := reg.Build(ctx, cfg)
	if err != nil {
		return SecretsParts{}, fmt.Errorf("secrets: %w", err)
	}
	repo := secrets.NewPostgresRepository(db)
	return SecretsParts{Provider: provider, Repo: repo, Manager: secrets.NewEnvelopeManager(provider, repo, secrets.AESGCMCipher{})}, nil
}

// SecretsAction names `api secrets` actions.
type SecretsAction string

const (
	SecretsRotate SecretsAction = "rotate"
	SecretsStatus SecretsAction = "status"
)

// SecretsCLI names fixed CLI texts and the audit actor of CLI runs.
type SecretsCLI string

const (
	secretsUsage    SecretsCLI = "usage: api secrets rotate | status"
	SecretsCLIActor SecretsCLI = "cli:secrets-rotate"
)

// SecretsCommand implements `api secrets rotate|status` (FR-29): re-wrap data keys to the current master key.
type SecretsCommand struct {
	out     io.Writer
	actions map[SecretsAction]func(ctx context.Context, r secrets.KeyRotator) error
}

// NewSecretsCommand builds the command writing to out.
func NewSecretsCommand(out io.Writer) *SecretsCommand {
	c := &SecretsCommand{out: out}
	c.actions = map[SecretsAction]func(context.Context, secrets.KeyRotator) error{SecretsRotate: c.rotate, SecretsStatus: c.status}
	return c
}

// Run executes the action named in args[0].
func (c *SecretsCommand) Run(args []string) app.ExitCode {
	if len(args) == 0 {
		fmt.Fprintln(c.out, secretsUsage)
		return app.ExitFailure
	}
	action, ok := c.actions[SecretsAction(args[0])]
	if !ok {
		fmt.Fprintln(c.out, secretsUsage)
		return app.ExitFailure
	}
	if err := c.execute(action); err != nil {
		fmt.Fprintln(c.out, "secrets:", err)
		return app.ExitFailure
	}
	return app.ExitOK
}

func (c *SecretsCommand) execute(action func(context.Context, secrets.KeyRotator) error) error {
	ctx := context.Background()
	cfg, err := config.NewLoader().Load(config.ServiceAPI)
	if err != nil {
		return err
	}
	db, err := store.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	parts, err := SecretsAssembly{}.Build(ctx, cfg.Secrets, db)
	if err != nil {
		return err
	}
	actor := audit.Actor{Kind: audit.ActorSystem, Name: string(SecretsCLIActor)}
	return action(ctx, parts.Rotator(audit.NewPostgresRecorder(db), actor, cfg.Secrets.Rotation))
}

func (c *SecretsCommand) rotate(ctx context.Context, r secrets.KeyRotator) error {
	report, err := r.Run(ctx)
	fmt.Fprintf(c.out, "rotated=%d raced=%d to_key_id=%s from_key_ids=%v\n", report.Rotated, report.Raced, report.ToKeyID, report.FromKeyIDs)
	if err != nil {
		return err
	}
	return c.status(ctx, r)
}

func (c *SecretsCommand) status(ctx context.Context, r secrets.KeyRotator) error {
	usage, err := r.Usage(ctx)
	if err != nil {
		return err
	}
	for _, u := range usage {
		fmt.Fprintf(c.out, "provider=%s key_id=%s envelopes=%d\n", u.Provider, u.KeyID, u.Count)
	}
	return nil
}
