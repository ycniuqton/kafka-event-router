package dlq

import (
	"fmt"

	"github.com/eventrouter/router/internal/platform/config"
)

// Env keys of the dead-letters feature (read by cmd/api).
const (
	EnvDLQStateBackend  config.EnvKey = "ROUTER_DLQ_STATE_BACKEND"
	EnvDLQRetryExecutor config.EnvKey = "ROUTER_DLQ_RETRY_EXECUTOR"
	EnvDLQScripts       config.EnvKey = "ROUTER_DLQ_SCRIPTS"
	EnvDLQJobStore      config.EnvKey = "ROUTER_DLQ_JOB_STORE"
	EnvDLQDemoSeed      config.EnvKey = "ROUTER_DLQ_DEMO_SEED"
)

// JobStore selects where bulk jobs are queued.
type JobStore string

const (
	JobStorePostgres JobStore = "postgres"
	JobStoreMemory   JobStore = "memory"
)

// ScriptsMode selects whether retries run JS steps in the sandbox.
type ScriptsMode string

const (
	ScriptsSandbox ScriptsMode = "sandbox"
	ScriptsNone    ScriptsMode = "none"
)

// Settings are the feature's configuration (design §2.5).
type Settings struct {
	StateBackend StateBackend
	Executor     ExecutorKind
	Scripts      ScriptsMode
	JobStore     JobStore
	DemoSeed     DemoSeedMode
}

// DefaultSettings returns the defaults: kafka state, in-process executor, sandboxed JS.
func DefaultSettings() Settings {
	return Settings{StateBackend: StateBackendKafka, Executor: ExecutorInProcess, Scripts: ScriptsSandbox, JobStore: JobStorePostgres, DemoSeed: DemoSeedOn}
}

// SettingsLoader reads settings through a lookup (os.LookupEnv in production).
type SettingsLoader struct {
	Lookup func(string) (string, bool)
}

// Load applies env overrides to the defaults and validates the values.
func (l SettingsLoader) Load() (Settings, error) {
	s := DefaultSettings()
	if v, ok := l.Lookup(string(EnvDLQStateBackend)); ok && v != "" {
		s.StateBackend = StateBackend(v)
	}
	if v, ok := l.Lookup(string(EnvDLQRetryExecutor)); ok && v != "" {
		s.Executor = ExecutorKind(v)
	}
	if v, ok := l.Lookup(string(EnvDLQScripts)); ok && v != "" {
		s.Scripts = ScriptsMode(v)
	}
	if v, ok := l.Lookup(string(EnvDLQJobStore)); ok && v != "" {
		s.JobStore = JobStore(v)
	}
	if v, ok := l.Lookup(string(EnvDLQDemoSeed)); ok && v != "" {
		s.DemoSeed = DemoSeedMode(v)
	}
	return s, l.validate(s)
}

func (SettingsLoader) validate(s Settings) error {
	known := map[string]bool{string(StateBackendKafka): true, string(StateBackendMemory): true}
	if !known[string(s.StateBackend)] {
		return fmt.Errorf("dlq: %s=%q is not one of kafka, memory", EnvDLQStateBackend, s.StateBackend)
	}
	if s.Scripts != ScriptsSandbox && s.Scripts != ScriptsNone {
		return fmt.Errorf("dlq: %s=%q is not one of sandbox, none", EnvDLQScripts, s.Scripts)
	}
	if s.JobStore != JobStorePostgres && s.JobStore != JobStoreMemory {
		return fmt.Errorf("dlq: %s=%q is not one of postgres, memory", EnvDLQJobStore, s.JobStore)
	}
	if s.DemoSeed != DemoSeedOn && s.DemoSeed != DemoSeedOff {
		return fmt.Errorf("dlq: %s=%q is not one of seed, none", EnvDLQDemoSeed, s.DemoSeed)
	}
	return nil
}
