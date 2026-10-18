// Package assembly composes the routes feature (repositories, validators, managers and the HTTP
// module) from shared infrastructure; cmd/api calls Build once and mounts Components.Module.
package assembly

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/eventrouter/router/internal/routes"
)

// SettingKey names the environment variables of the routes feature.
type SettingKey string

const (
	EnvApprovalPolicy     SettingKey = "ROUTER_APPROVAL_POLICY"
	EnvMetricsReader      SettingKey = "ROUTER_ROUTE_METRICS_READER"
	EnvScriptCheck        SettingKey = "ROUTER_ROUTES_SCRIPT_CHECK"
	EnvValidationTimeout  SettingKey = "ROUTER_ROUTES_VALIDATION_TIMEOUT"
	EnvValidationParallel SettingKey = "ROUTER_ROUTES_VALIDATION_PARALLELISM"
	EnvRolloutNotifier    SettingKey = "ROUTER_ROUTES_ROLLOUT_NOTIFIER"
	EnvReplayWindow       SettingKey = "ROUTER_ROUTES_REPLAY_WINDOW"
)

// ScriptCheckKind selects how JS steps are compiled during validation.
type ScriptCheckKind string

const (
	ScriptCheckSandbox ScriptCheckKind = "sandbox"
	ScriptCheckSkip    ScriptCheckKind = "skip"
)

// LookupFunc reads one environment variable.
type LookupFunc func(key string) (string, bool)

// Settings are the routes feature's configuration.
type Settings struct {
	ApprovalPolicy     routes.ApprovalPolicyKind
	MetricsReader      routes.MetricsReaderKind
	ScriptCheck        ScriptCheckKind
	ValidationTimeout  time.Duration
	ValidationParallel int
	RolloutNotifier    routes.RolloutNotifierKind
	ReplayWindow       routes.ReplayWindow
}

// SettingsLoader reads Settings with documented defaults.
type SettingsLoader struct {
	Lookup LookupFunc
}

// NewSettingsLoader reads from the process environment.
func NewSettingsLoader() SettingsLoader {
	return SettingsLoader{Lookup: os.LookupEnv}
}

// Load returns the settings or an error naming the malformed key.
func (l SettingsLoader) Load() (Settings, error) {
	s := Settings{
		ApprovalPolicy:  routes.ApprovalPolicyKind(l.str(EnvApprovalPolicy, string(routes.ApprovalAuto))),
		MetricsReader:   routes.MetricsReaderKind(l.str(EnvMetricsReader, string(routes.MetricsReaderNull))),
		ScriptCheck:     ScriptCheckKind(l.str(EnvScriptCheck, string(ScriptCheckSandbox))),
		RolloutNotifier: routes.RolloutNotifierKind(l.str(EnvRolloutNotifier, string(routes.RolloutNotifierLog))),
	}
	timeout, err := time.ParseDuration(l.str(EnvValidationTimeout, (5 * time.Second).String()))
	if err != nil {
		return Settings{}, fmt.Errorf("%s: %w", EnvValidationTimeout, err)
	}
	parallel, err := strconv.Atoi(l.str(EnvValidationParallel, "4"))
	if err != nil {
		return Settings{}, fmt.Errorf("%s: %w", EnvValidationParallel, err)
	}
	window, err := time.ParseDuration(l.str(EnvReplayWindow, time.Duration(routes.DefaultReplayWindow).String()))
	if err != nil {
		return Settings{}, fmt.Errorf("%s: %w", EnvReplayWindow, err)
	}
	s.ValidationTimeout, s.ValidationParallel, s.ReplayWindow = timeout, parallel, routes.ReplayWindow(window)
	return s, nil
}

func (l SettingsLoader) str(key SettingKey, fallback string) string {
	if v, ok := l.Lookup(string(key)); ok && v != "" {
		return v
	}
	return fallback
}
