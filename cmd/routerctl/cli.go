package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/config/configtest"
	"github.com/eventrouter/router/pkg/config/validators"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/batch"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
)

// ExitCode is the process exit status of routerctl.
type ExitCode int

const (
	ExitOK            ExitCode = 0
	ExitEventErrors   ExitCode = 1
	ExitInvalidConfig ExitCode = 2
	ExitUsage         ExitCode = 3
	ExitIO            ExitCode = 4
)

// Command names a routerctl subcommand.
type Command string

const CommandRun Command = "run"

// ScriptRuntimeFactory creates the JS runtime selected by --js-engine.
type ScriptRuntimeFactory func() (script.Runtime, error)

// CLI is the routerctl application; ScriptRuntimes is where js-sandbox registers its engine.
type CLI struct {
	stdout, stderr io.Writer
	ScriptRuntimes map[string]ScriptRuntimeFactory
}

// NewCLI builds the CLI with the registered script runtimes.
func NewCLI(stdout, stderr io.Writer) *CLI {
	return &CLI{stdout: stdout, stderr: stderr, ScriptRuntimes: map[string]ScriptRuntimeFactory{}}
}

// RunOptions are the flags of `routerctl run`.
type RunOptions struct {
	ConfigPath string
	EventsPath string
	Trace      bool
	Now        time.Time
	Library    string
	JSEngine   string
	TeamID     string
}

// Main dispatches the subcommand and returns the exit code.
func (c *CLI) Main(ctx context.Context, args []string) ExitCode {
	if len(args) == 0 || Command(args[0]) != CommandRun {
		fmt.Fprintln(c.stderr, "usage: routerctl run --config route.json --events events.jsonl [--trace] [--now RFC3339] [--library dir] [--js-engine name]")
		return ExitUsage
	}
	opts, err := c.parseRun(args[1:])
	if err != nil {
		fmt.Fprintln(c.stderr, err)
		return ExitUsage
	}
	return c.run(ctx, opts)
}

func (c *CLI) parseRun(args []string) (RunOptions, error) {
	fs := flag.NewFlagSet(string(CommandRun), flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	o := RunOptions{}
	now := fs.String("now", "", "frozen processing time (RFC 3339); default: current time")
	fs.StringVar(&o.ConfigPath, "config", "", "route config JSON file")
	fs.StringVar(&o.EventsPath, "events", "", "JSONL events file (- for stdin)")
	fs.BoolVar(&o.Trace, "trace", false, "write per-event trace JSON to stderr")
	fs.StringVar(&o.Library, "library", "", "library directory: transforms <ref>@<version>.json, lookup tables tables/<ref>@<version>.json")
	fs.StringVar(&o.JSEngine, "js-engine", "", "script runtime for js steps and expr filters")
	fs.StringVar(&o.TeamID, "team", "", "team id used for the hash salt scope")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if o.ConfigPath == "" || o.EventsPath == "" {
		return o, errors.New("--config and --events are required")
	}
	o.Now = time.Now().UTC()
	if *now != "" {
		t, err := time.Parse(time.RFC3339Nano, *now)
		if err != nil {
			return o, fmt.Errorf("--now: %w", err)
		}
		o.Now = t
	}
	return o, nil
}

// run validates, compiles and streams the events through the pipeline.
func (c *CLI) run(ctx context.Context, o RunOptions) ExitCode {
	raw, err := os.ReadFile(o.ConfigPath)
	if err != nil {
		fmt.Fprintln(c.stderr, err)
		return ExitIO
	}
	copts, err := c.compileOptions(o)
	if err != nil {
		fmt.Fprintln(c.stderr, err)
		return ExitUsage
	}
	doc, code := c.validate(ctx, raw, copts)
	if code != ExitOK {
		return code
	}
	p, err := compile.NewDefaultCompiler().Compile(ctx, doc, copts)
	if err != nil {
		fmt.Fprintln(c.stderr, err)
		return ExitInvalidConfig
	}
	defer p.Close(ctx)
	return c.stream(ctx, o, doc, p)
}

func (c *CLI) compileOptions(o RunOptions) (engine.CompileOptions, error) {
	opts := engine.CompileOptions{Meta: engine.RouteMeta{TeamID: o.TeamID}}
	opts.Helpers = ctxhelpers.Set{Clock: ctxhelpers.FixedClock{T: o.Now}}.WithDefaults()
	if o.JSEngine != "" {
		factory, ok := c.ScriptRuntimes[o.JSEngine]
		if !ok {
			return opts, fmt.Errorf("--js-engine %q is not registered", o.JSEngine)
		}
		rt, err := factory()
		if err != nil {
			return opts, err
		}
		opts.Scripts = rt
	}
	if o.Library != "" {
		lib, err := configtest.LibraryDir{Path: o.Library}.Load()
		if err != nil {
			return opts, err
		}
		opts.Library = lib
	}
	return opts, nil
}

// validate runs the offline profile and prints issues as JSONL on stderr.
func (c *CLI) validate(ctx context.Context, raw []byte, copts engine.CompileOptions) (*config.Document, ExitCode) {
	m, err := validators.NewDefaultManager(validators.Deps{Compiler: compile.NewDefaultCompiler(), Options: copts})
	if err != nil {
		fmt.Fprintln(c.stderr, err)
		return nil, ExitInvalidConfig
	}
	issues, err := m.Validate(ctx, config.StageOffline, copts.Meta.TeamID, raw)
	if err != nil {
		fmt.Fprintln(c.stderr, err)
		return nil, ExitInvalidConfig
	}
	enc := json.NewEncoder(c.stderr)
	for _, is := range issues {
		_ = enc.Encode(is)
	}
	if issues.HasErrors() {
		return nil, ExitInvalidConfig
	}
	doc, err := config.Parse(raw)
	if err != nil {
		return nil, ExitInvalidConfig
	}
	return doc, ExitOK
}

func (c *CLI) stream(ctx context.Context, o RunOptions, doc *config.Document, p engine.Pipeline) ExitCode {
	in, closeIn, err := InputFile{Path: o.EventsPath}.Open()
	if err != nil {
		fmt.Fprintln(c.stderr, err)
		return ExitIO
	}
	defer closeIn()
	cfg, _ := doc.Config()
	sinks := batch.Sinks{Outputs: c.stdout, Errors: c.stderr}
	if o.Trace {
		sinks.Traces = c.stderr
	}
	sum, err := batch.Runner{Pipeline: p, Topic: SourceTopic{}.Of(cfg), Timestamp: o.Now}.Run(ctx, in, sinks)
	if err != nil {
		fmt.Fprintln(c.stderr, err)
		return ExitIO
	}
	if sum.Errors > 0 {
		return ExitEventErrors
	}
	return ExitOK
}
