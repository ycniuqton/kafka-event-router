package connections

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/eventrouter/router/internal/platform/config"
)

// Configuration keys of the connections feature.
const (
	EnvProbeInterval config.EnvKey = "ROUTER_CONNECTION_PROBE_INTERVAL"
	EnvTestSteps     config.EnvKey = "ROUTER_CONNECTION_TEST_STEPS"
	EnvProbeEnabled  config.EnvKey = "ROUTER_CONNECTION_PROBE_ENABLED"
)

// ConfigDefaults are the fallback settings.
type ConfigDefaults time.Duration

const DefaultProbeInterval ConfigDefaults = ConfigDefaults(60 * time.Second)

// Settings is the feature configuration read from the environment.
type Settings struct {
	ProbeInterval time.Duration
	ProbeEnabled  bool
	TestPlan      CheckPlan
}

// SettingsLoader reads Settings through a lookup function (os.LookupEnv in production).
type SettingsLoader struct {
	Lookup config.LookupFunc
}

// Load returns settings, rejecting malformed values.
func (l SettingsLoader) Load() (Settings, error) {
	s := Settings{ProbeInterval: time.Duration(DefaultProbeInterval), ProbeEnabled: true, TestPlan: DefaultTestPlan}
	if raw, ok := l.Lookup(string(EnvProbeInterval)); ok && raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return s, fmt.Errorf("%s: invalid duration %q", EnvProbeInterval, raw)
		}
		s.ProbeInterval = d
	}
	if raw, ok := l.Lookup(string(EnvProbeEnabled)); ok && raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return s, fmt.Errorf("%s: invalid boolean %q", EnvProbeEnabled, raw)
		}
		s.ProbeEnabled = enabled
	}
	if raw, ok := l.Lookup(string(EnvTestSteps)); ok && raw != "" {
		s.TestPlan = l.plan(raw)
	}
	return s, nil
}

func (l SettingsLoader) plan(raw string) CheckPlan {
	var plan CheckPlan
	for _, part := range strings.Split(raw, string(config.CommaSeparator)) {
		if step := TestStep(strings.TrimSpace(part)); step != "" {
			plan = append(plan, step)
		}
	}
	return plan
}
