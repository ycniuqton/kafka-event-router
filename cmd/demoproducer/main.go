// Command demoproducer feeds the staging demo topics with a low, steady rate of realistic JSON events
// (orders on cluster A, payments / raw readings / local events on cluster B) so the demo routes stay alive.
//
//	demoproducer          run the feed (configured by DEMO_* environment variables, see FeedConfig)
//	demoproducer health   exit 0 when the running feed produced recently (container healthcheck)
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// ExitCodes of the process.
var ExitCodes = struct{ OK, Failed int }{OK: 0, Failed: 1}

// Command is one demoproducer mode.
type Command interface {
	Run(ctx context.Context, cfg FeedConfig) error
}

// Commands maps the first argument to a mode; no argument runs the feed.
var Commands = map[string]Command{
	"":       FeedCommand{},
	"health": HealthCommand{},
}

func main() {
	os.Exit(App{}.Run(os.Args[1:]))
}

// App resolves the mode and runs it under a context cancelled by SIGINT/SIGTERM.
type App struct{}

// Run executes the command line and returns the exit code.
func (App) Run(args []string) int {
	mode := ""
	if len(args) > 0 {
		mode = args[0]
	}
	cmd, ok := Commands[mode]
	if !ok {
		fmt.Fprintln(os.Stderr, "usage: demoproducer [health]")
		return ExitCodes.Failed
	}
	cfg, err := FeedConfigFromEnv(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "demoproducer:", err)
		return ExitCodes.Failed
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cmd.Run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "demoproducer:", err)
		return ExitCodes.Failed
	}
	return ExitCodes.OK
}
