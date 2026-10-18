// Command loadgen generates and measures stress load for the event router.
//
//	loadgen produce --brokers 127.0.0.1:39092 --topic stress-x-in --rate 20000 --duration 5m --size 1024
//	loadgen measure --brokers 127.0.0.1:39092 --topics stress-x-out --out result.json
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"
)

// Command is one loadgen subcommand.
type Command interface {
	Usage() string
	Run(ctx context.Context, args []string) error
}

// Commands is the registry of subcommands.
var Commands = map[string]Command{
	"produce": &ProduceCommand{},
	"measure": &MeasureCommand{},
}

// ExitCodes of the process.
var ExitCodes = struct{ OK, Failed, Usage int }{OK: 0, Failed: 1, Usage: 2}

func main() {
	os.Exit(App{}.Run(os.Args[1:]))
}

// App dispatches a subcommand under a context cancelled by SIGINT/SIGTERM (and stdin EOF when asked).
type App struct{}

// Run executes the command line and returns the exit code.
func (App) Run(args []string) int {
	if len(args) == 0 || Commands[args[0]] == nil {
		App{}.usage()
		return ExitCodes.Usage
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx = StopOnEOF{}.Watch(ctx, args[1:])
	if err := Commands[args[0]].Run(ctx, args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "loadgen:", err)
		return ExitCodes.Failed
	}
	return ExitCodes.OK
}

func (App) usage() {
	names := make([]string, 0, len(Commands))
	for n := range Commands {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintln(os.Stderr, "usage: loadgen <command> [flags]   (-h on a command for its flags)")
	for _, n := range names {
		fmt.Fprintf(os.Stderr, "  %-8s %s\n", n, Commands[n].Usage())
	}
}

// StopOnEOF cancels the run when stdin closes; the runner uses it to stop a loadgen started through ssh, where
// signals do not reach the remote process.
type StopOnEOF struct{}

// Watch returns ctx, cancelled at stdin EOF when the flag is present.
func (StopOnEOF) Watch(ctx context.Context, args []string) context.Context {
	for _, a := range args {
		if a == string(SwitchStopOnEOF) || a == string(SwitchStopOnEOF)+"=true" {
			ctx, cancel := context.WithCancel(ctx)
			go func() {
				_, _ = io.Copy(io.Discard, os.Stdin)
				cancel()
			}()
			return ctx
		}
	}
	return ctx
}
