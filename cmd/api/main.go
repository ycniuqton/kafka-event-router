// Command api runs the control-plane REST/WS API (public listener) and the internal workload
// listener, and offers `api migrate up|down N|status` `api secrets rotate|status` and `api health <url>`.
package main

import (
	"os"

	"github.com/eventrouter/router/internal/platform/app"
)

// Subcommand names CLI subcommands of the api binary.
type Subcommand string

const (
	SubcommandMigrate Subcommand = "migrate"
	SubcommandSecrets Subcommand = "secrets"
)

// ArgIndex names positions in os.Args.
type ArgIndex int

const (
	ArgSubcommand ArgIndex = 1
	ArgRest       ArgIndex = 2
)

// CLICommand is the base of the api binary's subcommands.
type CLICommand interface {
	Run(args []string) app.ExitCode
}

// cliCommands maps subcommand names to their implementations.
func cliCommands() map[Subcommand]CLICommand {
	return map[Subcommand]CLICommand{
		SubcommandMigrate: NewMigrateCommand(os.Stdout),
		SubcommandSecrets: NewSecretsCommand(os.Stdout),
		SubcommandHealth:  NewHealthCommand(os.Stdout),
	}
}

func main() {
	if len(os.Args) > int(ArgSubcommand) {
		if cmd, ok := cliCommands()[Subcommand(os.Args[ArgSubcommand])]; ok {
			os.Exit(int(cmd.Run(os.Args[ArgRest:])))
		}
	}
	os.Exit(int(app.NewRunner(newAPIService()).Run()))
}
