// Command stresssweep removes leftovers of interrupted stress runs: it archives routes, deletes connections,
// topics and consumer groups whose name starts with --prefix (which must start with stress-).
//
//	stresssweep --prefix stress-s3- [--keep stress-local] [--dry-run]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/eventrouter/router/test/stress/kit/kafkaops"
	"github.com/eventrouter/router/test/stress/kit/platform"
	"github.com/eventrouter/router/test/stress/kit/scenario"
)

// Defaults of the sweep command.
var Defaults = struct {
	Targets, Target string
	Timeout         time.Duration
	Rounds          int
}{Targets: "test/stress/targets.yaml", Target: "local", Timeout: 30 * time.Minute, Rounds: 50}

func main() {
	if err := (Sweep{}).Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "stresssweep:", err)
		os.Exit(1)
	}
}

// Sweep is the stresssweep command.
type Sweep struct {
	prefix string
	keep   map[string]bool
	dry    bool
	plat   *platform.Platform
	adm    *kafkaops.Admin
}

// Run parses flags, connects to the target and sweeps routes, connections, topics and groups.
func (s Sweep) Run(args []string) error {
	target, err := s.parse(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), Defaults.Timeout)
	defer cancel()
	if err := s.connect(ctx, target); err != nil {
		return err
	}
	defer s.adm.Close()
	return errors.Join(s.routes(ctx), s.connections(ctx), s.kafka(ctx))
}

func (s *Sweep) parse(args []string) (string, error) {
	fs := flag.NewFlagSet("stresssweep", flag.ContinueOnError)
	target := fs.String("target", Defaults.Target, "target name in "+Defaults.Targets)
	keep := fs.String("keep", "", "comma-separated names to keep (e.g. the shared stress-local connection)")
	fs.StringVar(&s.prefix, "prefix", "", "name prefix to sweep (must start with "+kafkaops.StressGuard.Prefix+")")
	fs.BoolVar(&s.dry, "dry-run", false, "only list what would be removed")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	s.keep = map[string]bool{}
	for _, k := range strings.Split(*keep, ",") {
		s.keep[strings.TrimSpace(k)] = true
	}
	return *target, kafkaops.StressGuard.Check(s.prefix)
}

func (s *Sweep) connect(ctx context.Context, name string) error {
	tg, err := scenario.TargetLoader{}.Load(Defaults.Targets, name)
	if err != nil {
		return err
	}
	client, err := platform.LoginManager{}.SignIn(ctx, tg.API)
	if err != nil {
		return err
	}
	if s.plat, err = platform.NewPlatform(ctx, client); err != nil {
		return err
	}
	s.adm, err = kafkaops.Dial(strings.Split(tg.Kafka.RunnerBootstrap, ","))
	return err
}

// routes archives matching routes page by page until none is left.
func (s *Sweep) routes(ctx context.Context) error {
	var errs []error
	for round := 0; round < Defaults.Rounds; round++ {
		items, err := s.plat.ListPrefixed(ctx, platform.Paths.Routes, s.prefix)
		if err != nil || len(items) == 0 || s.dry {
			s.report("routes", items)
			return errors.Join(append(errs, err)...)
		}
		for _, it := range items {
			errs = append(errs, s.plat.Archive(ctx, it.ID))
		}
		fmt.Printf("archived %d route(s)\n", len(items))
	}
	return errors.Join(errs...)
}

func (s *Sweep) connections(ctx context.Context) error {
	items, err := s.plat.ListPrefixed(ctx, platform.Paths.Connections, s.prefix)
	if err != nil {
		return err
	}
	var errs []error
	for _, it := range items {
		if s.keep[it.Name] || s.dry {
			continue
		}
		errs = append(errs, s.plat.DeleteConnection(ctx, it.ID))
		fmt.Printf("deleted connection %s\n", it.Name)
	}
	return errors.Join(errs...)
}

func (s *Sweep) kafka(ctx context.Context) error {
	topics, err := s.adm.ListGuarded(ctx, s.prefix)
	if err != nil {
		return err
	}
	groups, err := s.adm.ListGroupsGuarded(ctx, s.prefix)
	if err != nil {
		return err
	}
	topics, groups = s.without(topics), s.without(groups)
	fmt.Printf("topics: %d, groups: %d\n", len(topics), len(groups))
	if s.dry {
		return nil
	}
	return errors.Join(s.adm.DeleteGroups(ctx, groups...), s.adm.DeleteTopics(ctx, topics...))
}

func (s *Sweep) without(names []string) []string {
	var out []string
	for _, n := range names {
		if !s.keep[n] {
			out = append(out, n)
		}
	}
	return out
}

func (s *Sweep) report(what string, items []platform.Named) {
	fmt.Printf("%s left: %d\n", what, len(items))
}
