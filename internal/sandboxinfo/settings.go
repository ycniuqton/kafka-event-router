// Package sandboxinfo binds the JS sandbox to the services: env settings, the runtime-info endpoint
// for the Studio editor and the per-team hash salt provisioning over internal/secrets.
package sandboxinfo

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/pkg/sandbox"
)

// Env keys of the sandbox (js-sandbox design §4), read by every service that runs JS.
const (
	EnvEngine       config.EnvKey = "ROUTER_SANDBOX_ENGINE"
	EnvMaxTimeoutMs config.EnvKey = "ROUTER_SANDBOX_MAX_TIMEOUT_MS"
	EnvMaxMemoryMb  config.EnvKey = "ROUTER_SANDBOX_MAX_MEMORY_MB"
	EnvMaxOutputKb  config.EnvKey = "ROUTER_SANDBOX_MAX_OUTPUT_KB"
	EnvResetPolicy  config.EnvKey = "ROUTER_SANDBOX_RESET_POLICY"
	EnvPoolMaxIdle  config.EnvKey = "ROUTER_SANDBOX_POOL_MAX_IDLE"
	EnvCacheDir     config.EnvKey = "ROUTER_SANDBOX_CACHE_DIR"
	EnvPrograms     config.EnvKey = "ROUTER_SANDBOX_PROGRAM_CACHE"
)

// SettingsLoader reads sandbox.Config from the environment on top of sandbox.DefaultConfig.
type SettingsLoader struct {
	Lookup config.LookupFunc
}

// NewSettingsLoader reads from the process environment.
func NewSettingsLoader() SettingsLoader {
	return SettingsLoader{Lookup: os.LookupEnv}
}

// Load returns the configured sandbox settings; engine kind and reset policy are checked by the Manager.
func (l SettingsLoader) Load() (sandbox.Config, error) {
	cfg := sandbox.DefaultConfig()
	cfg.Engine = sandbox.EngineKind(l.str(EnvEngine, string(cfg.Engine)))
	cfg.ResetPolicy = sandbox.ResetPolicy(l.str(EnvResetPolicy, string(cfg.ResetPolicy)))
	cfg.CacheDir = l.str(EnvCacheDir, cfg.CacheDir)
	cfg.Programs = sandbox.ProgramCacheKind(l.str(EnvPrograms, string(cfg.Programs)))
	steps := []SettingStep{
		{Key: EnvMaxTimeoutMs, Unit: int64(time.Millisecond), Target: (*int64)(&cfg.Maxima.Timeout)},
		{Key: EnvMaxMemoryMb, Unit: int64(sandbox.MiB), Target: &cfg.Maxima.MemoryBytes},
		{Key: EnvMaxOutputKb, Unit: int64(sandbox.KiB), Target: &cfg.Maxima.MaxOutputBytes},
	}
	for _, s := range steps {
		if err := l.apply(s); err != nil {
			return sandbox.Config{}, err
		}
	}
	idle := int64(cfg.PoolMaxIdle)
	if err := l.apply(SettingStep{Key: EnvPoolMaxIdle, Unit: 1, Target: &idle}); err != nil {
		return sandbox.Config{}, err
	}
	cfg.PoolMaxIdle = int(idle)
	return cfg, nil
}

// SettingStep scales one positive integer env value by Unit into Target (left unchanged when unset).
type SettingStep struct {
	Key    config.EnvKey
	Unit   int64
	Target *int64
}

func (l SettingsLoader) apply(s SettingStep) error {
	v, ok := l.Lookup(string(s.Key))
	if !ok || v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("sandbox: %s must be a positive integer, got %q", s.Key, v)
	}
	*s.Target = n * s.Unit
	return nil
}

func (l SettingsLoader) str(key config.EnvKey, def string) string {
	if v, ok := l.Lookup(string(key)); ok && v != "" {
		return v
	}
	return def
}

// LogEvent names the log messages of the sandbox wiring.
type LogEvent string

const (
	LogSettingsInvalid LogEvent = "sandbox settings invalid; using defaults"
	LogModuleDisabled  LogEvent = "sandbox runtime-info module disabled"
	LogMetricsOff      LogEvent = "sandbox metrics unavailable; js calls are not observed"
)
