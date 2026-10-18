package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/eventrouter/router/internal/platform/app"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/store"
)

// MigrateAction names `api migrate` actions.
type MigrateAction string

const (
	MigrateUp     MigrateAction = "up"
	MigrateDown   MigrateAction = "down"
	MigrateStatus MigrateAction = "status"
)

// DefaultDownSteps is how many migrations `down` reverts without an explicit count.
type DownSteps int

const DefaultDownSteps DownSteps = 1

// MigrateUsage is printed for invalid invocations.
type MigrateUsage string

const migrateUsage MigrateUsage = "usage: api migrate up | down [N] | status"

// MigrateCommand implements `api migrate up|down N|status`.
type MigrateCommand struct {
	out     io.Writer
	actions map[MigrateAction]func(ctx context.Context, m *store.Migrator, args []string) error
}

// NewMigrateCommand builds the command writing to out.
func NewMigrateCommand(out io.Writer) *MigrateCommand {
	c := &MigrateCommand{out: out}
	c.actions = map[MigrateAction]func(context.Context, *store.Migrator, []string) error{
		MigrateUp:     c.up,
		MigrateDown:   c.down,
		MigrateStatus: c.status,
	}
	return c
}

// Run executes the action named in args[0].
func (c *MigrateCommand) Run(args []string) app.ExitCode {
	if len(args) == 0 {
		fmt.Fprintln(c.out, migrateUsage)
		return app.ExitFailure
	}
	action, ok := c.actions[MigrateAction(args[0])]
	if !ok {
		fmt.Fprintln(c.out, migrateUsage)
		return app.ExitFailure
	}
	if err := c.execute(action, args[1:]); err != nil {
		fmt.Fprintln(c.out, "migrate:", err)
		return app.ExitFailure
	}
	return app.ExitOK
}

func (c *MigrateCommand) execute(action func(context.Context, *store.Migrator, []string) error, args []string) error {
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
	return action(ctx, store.NewMigrator(db, cfg.Database.URL), args)
}

func (c *MigrateCommand) up(ctx context.Context, m *store.Migrator, _ []string) error {
	if err := m.Up(ctx); err != nil {
		return err
	}
	return c.status(ctx, m, nil)
}

func (c *MigrateCommand) down(ctx context.Context, m *store.Migrator, args []string) error {
	steps := int(DefaultDownSteps)
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n <= 0 {
			return errors.New("down needs a positive number of steps")
		}
		steps = n
	}
	if err := m.Down(ctx, steps); err != nil {
		return err
	}
	return c.status(ctx, m, nil)
}

func (c *MigrateCommand) status(ctx context.Context, m *store.Migrator, _ []string) error {
	st, err := m.Status(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "applied=%d latest=%d dirty=%t\n", st.Applied, st.Latest, st.Dirty)
	return nil
}
