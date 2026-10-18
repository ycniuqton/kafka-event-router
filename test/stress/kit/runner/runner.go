package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/eventrouter/router/test/stress/kit/kafkaops"
	"github.com/eventrouter/router/test/stress/kit/platform"
	"github.com/eventrouter/router/test/stress/kit/sampling"
	"github.com/eventrouter/router/test/stress/kit/scenario"
)

// KafkaAdmin is the Kafka administration the runner needs.
type KafkaAdmin interface {
	EnsureTopics(ctx context.Context, partitions int32, names ...string) error
	DeleteTopics(ctx context.Context, names ...string) error
	DeleteGroups(ctx context.Context, groups ...string) error
}

// Sampling is the periodic observer of the system under test.
type Sampling interface {
	Track(routeIDs, groups []string)
	Sample(ctx context.Context) sampling.Snapshot
	PIDs(ctx context.Context) map[string]int
}

// Options are the command-line choices of one run.
type Options struct {
	ResultsDir string
	Cases      map[string]bool
	Keep       bool
	EnvTag     string
}

// CleanupRetry bounds consumer-group deletion after archiving (groups stay non-empty until workers let go).
var CleanupRetry = struct {
	Attempts int
	Pause    time.Duration
}{Attempts: 12, Pause: 5 * time.Second}

// ScenarioRunner runs every selected case of a scenario on a target.
type ScenarioRunner struct {
	Scenario scenario.Scenario
	Target   scenario.Target
	Options  Options
	Log      func(format string, args ...any)
}

// Run signs in, provisions, runs the cases, cleans up and writes results; returns the result file path.
func (r ScenarioRunner) Run(ctx context.Context) (string, error) {
	env, adm, err := r.connect(ctx)
	if err != nil {
		return "", err
	}
	defer adm.Close()
	result := ScenarioResult{
		Scenario: r.Scenario.Name, Description: r.Scenario.Description, Target: r.Target.Name, RunBase: env.Naming.Base,
		Host: NewHostInfo(), Criteria: r.Scenario.Criteria, StartedAt: time.Now(),
	}
	writer := ResultWriter{Dir: r.Options.ResultsDir}
	runRegistry := env.Registry
	connID, err := env.Platform.CreateConnection(ctx, env.Naming.Connection(), r.Target.Kafka.PlatformBootstrap, r.Options.EnvTag)
	if err != nil {
		return "", err
	}
	runRegistry.Connection(connID)
	env.ConnectionID = connID
	r.runCases(ctx, env, &result, writer)
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if !r.Options.Keep {
		rep := r.cleanup(cleanupCtx, env, runRegistry)
		result.Cleanup = &rep
	}
	result.FinishedAt = time.Now()
	return writer.Write(result)
}

func (r ScenarioRunner) runCases(ctx context.Context, env *Env, result *ScenarioResult, writer ResultWriter) {
	prov := &Provisioned{}
	runRegistry := env.Registry
	caseRegistry := &Registry{}
	for _, c := range r.Scenario.Cases {
		if ctx.Err() != nil || (len(r.Options.Cases) > 0 && !r.Options.Cases[c.Name]) {
			continue
		}
		if !r.Scenario.Incremental {
			prov, caseRegistry = &Provisioned{}, &Registry{}
		}
		env.Registry = caseRegistry
		r.Log("case %s: %d route(s), %d partition(s), %d step(s)", c.Name, c.Routes.Count, c.Partitions, len(c.Load.Plan().Steps))
		cr := (&CaseRunner{env: env, spec: c, prov: prov}).Run(ctx)
		result.Cases = append(result.Cases, cr)
		r.Log("case %s: max sustainable %.0f ev/s, knee %.0f (%s), lost %d, error %q", c.Name, cr.Knee.MaxSustainable, cr.Knee.KneeRate, cr.Knee.KneeReason, cr.Delivery.Lost, cr.Error)
		if !r.Scenario.Incremental && !r.Options.Keep {
			rep := r.cleanup(context.Background(), env, caseRegistry)
			result.Cases[len(result.Cases)-1].Cleanup = &rep
		}
		_, _ = writer.Write(*result)
	}
	if r.Scenario.Incremental && !r.Options.Keep && len(result.Cases) > 0 {
		rep := r.cleanup(context.Background(), env, caseRegistry)
		result.Cases[len(result.Cases)-1].Cleanup = &rep
	}
	env.Registry = runRegistry
}

func (r ScenarioRunner) connect(ctx context.Context) (*Env, *kafkaops.Admin, error) {
	client, err := platform.LoginManager{}.SignIn(ctx, r.Target.API)
	if err != nil {
		return nil, nil, fmt.Errorf("sign in to %s: %w", r.Target.API.BaseURL, err)
	}
	p, err := platform.NewPlatform(ctx, client)
	if err != nil {
		return nil, nil, err
	}
	adm, err := kafkaops.Dial(strings.Split(r.Target.Kafka.RunnerBootstrap, ","))
	if err != nil {
		return nil, nil, err
	}
	adm.WithTopicConfig(r.Target.Kafka.TopicConfig)
	smp, err := sampling.NewSampler(r.Target, adm)
	if err != nil {
		adm.Close()
		return nil, nil, err
	}
	CPUPinner{Log: r.Log}.Apply(ctx, r.Target.Processes, smp.PIDs(ctx))
	env := &Env{
		Scenario: r.Scenario, Target: r.Target, Platform: p, Kafka: adm, Sampler: smp, Registry: &Registry{}, Log: r.Log,
		Naming:   NewNaming(r.Scenario.Name),
		Launcher: Launcher{Spec: r.Target.Loadgen, LogDir: r.Options.ResultsDir + "/logs/" + r.Scenario.Name},
	}
	return env, adm, nil
}

func (r ScenarioRunner) cleanup(ctx context.Context, env *Env, reg *Registry) CleanupReport {
	rep := reg.Cleanup(ctx, env.Platform, env.Kafka)
	r.Log("cleanup: archived %d route(s), deleted %d topic(s), %d error(s)", rep.ArchivedRoutes, rep.DeletedTopics, len(rep.Errors))
	return rep
}

// CPUPinner applies taskset CPU affinity hints of target processes (all threads, -a).
type CPUPinner struct {
	Log func(string, ...any)
}

// Apply pins every process with a cpus hint whose pid is known.
func (c CPUPinner) Apply(ctx context.Context, specs []scenario.ProcessSpec, pids map[string]int) {
	for _, s := range specs {
		pid, ok := pids[s.Name]
		if s.CPUs == "" || !ok {
			continue
		}
		argv := append(append([]string{}, s.Reader[:max(len(s.Reader)-1, 0)]...), "taskset", "-acp", s.CPUs, strconv.Itoa(pid))
		out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
		if err != nil {
			c.Log("pin %s (pid %d) to %s failed: %v %s", s.Name, pid, s.CPUs, err, strings.TrimSpace(string(out)))
			continue
		}
		c.Log("pinned %s (pid %d) to CPUs %s", s.Name, pid, s.CPUs)
	}
}

// Stderr logger used by the command.
func StderrLog(format string, args ...any) {
	fmt.Fprintf(os.Stderr, time.Now().Format("15:04:05 ")+format+"\n", args...)
}
