// Command stressrun runs a stress scenario (test/stress/scenarios/*.yaml) against a target (test/stress/targets.yaml).
//
//	stressrun --scenario s2-single-route-ramp --target local
//	stressrun --scenario test/stress/scenarios/smoke.yaml --dry-run
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/eventrouter/router/test/stress/kit/platform"
	"github.com/eventrouter/router/test/stress/kit/runner"
	"github.com/eventrouter/router/test/stress/kit/scenario"
)

// Layout holds the default locations of stress files, relative to the repository root.
var Layout = struct {
	Scenarios, Targets, Results, Ext, EnvTag string
}{Scenarios: "test/stress/scenarios", Targets: "test/stress/targets.yaml", Results: "test/stress/results", Ext: ".yaml", EnvTag: "dev"}

func main() {
	if err := (App{}).Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "stressrun:", err)
		os.Exit(1)
	}
}

// App is the stressrun command.
type App struct{}

// Flags are the parsed command line.
type Flags struct {
	Scenario, Target, Targets, Results, Cases, EnvTag string
	Keep, DryRun                                      bool
}

// Run parses flags, loads scenario and target, then runs (or prints the plan with --dry-run).
func (a App) Run(args []string) error {
	f, err := a.parse(args)
	if err != nil {
		return err
	}
	sc, err := scenario.Loader{}.Load(a.scenarioPath(f.Scenario))
	if err != nil {
		return err
	}
	if f.Target == "" {
		f.Target = sc.Target
	}
	if f.DryRun {
		return DryRun{Cases: a.caseSet(f.Cases)}.Print(sc)
	}
	tg, err := scenario.TargetLoader{}.Load(f.Targets, f.Target)
	if err != nil {
		return err
	}
	tg = sc.WithExtras(tg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	opts := runner.Options{ResultsDir: f.Results, Cases: a.caseSet(f.Cases), Keep: f.Keep, EnvTag: f.EnvTag}
	path, err := runner.ScenarioRunner{Scenario: sc, Target: tg, Options: opts, Log: runner.StderrLog}.Run(ctx)
	if path != "" {
		fmt.Println(path)
	}
	return err
}

func (App) parse(args []string) (Flags, error) {
	var f Flags
	fs := flag.NewFlagSet("stressrun", flag.ContinueOnError)
	fs.StringVar(&f.Scenario, "scenario", "", "scenario name (in "+Layout.Scenarios+") or YAML path (required)")
	fs.StringVar(&f.Target, "target", "", "target name in the targets file: local|staging (default: the scenario's target)")
	fs.StringVar(&f.Targets, "targets", Layout.Targets, "targets file")
	fs.StringVar(&f.Results, "results", Layout.Results, "results directory")
	fs.StringVar(&f.Cases, "cases", "", "comma-separated case names to run (default all)")
	fs.StringVar(&f.EnvTag, "env-tag", Layout.EnvTag, "envTag of the stress connection (non-prod: no deploy approval)")
	fs.BoolVar(&f.Keep, "keep", false, "keep stress- resources (no archive/delete) for inspection")
	fs.BoolVar(&f.DryRun, "dry-run", false, "print the merged cases and route drafts, touch nothing")
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	if f.Scenario == "" {
		return f, fmt.Errorf("--scenario is required")
	}
	return f, nil
}

func (App) scenarioPath(s string) string {
	if strings.HasSuffix(s, Layout.Ext) || strings.ContainsRune(s, os.PathSeparator) {
		return s
	}
	return filepath.Join(Layout.Scenarios, s+Layout.Ext)
}

func (App) caseSet(list string) map[string]bool {
	out := map[string]bool{}
	for _, c := range strings.Split(list, ",") {
		if c = strings.TrimSpace(c); c != "" {
			out[c] = true
		}
	}
	return out
}

// DryRun prints what a run would do for the selected cases (all when Cases is empty).
type DryRun struct {
	Cases map[string]bool
}

// Print writes each merged case with the draft of its first route and its loadgen arguments.
func (d DryRun) Print(sc scenario.Scenario) error {
	names := runner.NewNaming(sc.Name)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	for _, c := range sc.Cases {
		if len(d.Cases) > 0 && !d.Cases[c.Name] {
			continue
		}
		topics := names.Routes(c.Name, c.Routes.Topology, 0, 1)
		draft, err := platform.DraftBuilder{Spec: c.Routes, ConnectionID: "<connection>"}.Build(topics[0])
		if err != nil {
			return fmt.Errorf("case %s: %w", c.Name, err)
		}
		out := map[string]any{
			"case": c, "plan": c.Load.Plan(), "firstRouteDraft": draft,
			"produceArgs": runner.LoadgenArgs{}.Produce("<bootstrap>", []string{topics[0].Input}, c.Load, 0, sc.SampleInterval),
		}
		if err := enc.Encode(out); err != nil {
			return err
		}
	}
	return nil
}
