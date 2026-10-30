package membudget

import (
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/eventrouter/router/internal/platform/config"
)

// Env keys of the worker memory budget (route-runtime 00002).
const (
	EnvBudgetBytes       config.EnvKey = "ROUTER_WORKER_MEMORY_BUDGET_BYTES"
	EnvBudgetPercent     config.EnvKey = "ROUTER_WORKER_MEMORY_BUDGET_PERCENT"
	EnvGoMemLimitPercent config.EnvKey = "ROUTER_WORKER_GOMEMLIMIT_PERCENT"
	EnvGoMemLimit        config.EnvKey = "GOMEMLIMIT"
)

// DefaultPercent names the default shares of the memory limit.
const (
	DefaultBudgetPercent     Watermark = 60
	DefaultGoMemLimitPercent Watermark = 90
)

// Settings configure the budget size and the automatic GOMEMLIMIT.
type Settings struct {
	BudgetBytes       int64
	BudgetPercent     Watermark
	GoMemLimitPercent Watermark
}

// SettingsLoader reads Settings from the environment.
type SettingsLoader struct {
	Lookup config.LookupFunc
}

// NewSettingsLoader reads from the process environment.
func NewSettingsLoader() SettingsLoader {
	return SettingsLoader{Lookup: os.LookupEnv}
}

// Load returns the settings; percentages must be 1..100 and the byte budget non-negative.
func (l SettingsLoader) Load() (Settings, error) {
	s := Settings{BudgetPercent: DefaultBudgetPercent, GoMemLimitPercent: DefaultGoMemLimitPercent}
	bytes, err := l.number(EnvBudgetBytes, 0, math.MaxInt64)
	if err != nil {
		return Settings{}, err
	}
	s.BudgetBytes = bytes
	for _, p := range []PercentSetting{{Key: EnvBudgetPercent, Target: &s.BudgetPercent}, {Key: EnvGoMemLimitPercent, Target: &s.GoMemLimitPercent}} {
		if err := l.percent(p); err != nil {
			return Settings{}, err
		}
	}
	return s, nil
}

// GoMemLimitSet reports whether the operator set GOMEMLIMIT (then it is never overridden).
func (l SettingsLoader) GoMemLimitSet() bool {
	v, ok := l.Lookup(string(EnvGoMemLimit))
	return ok && v != ""
}

// PercentSetting binds one percentage env key to its field.
type PercentSetting struct {
	Key    config.EnvKey
	Target *Watermark
}

func (l SettingsLoader) percent(p PercentSetting) error {
	n, err := l.number(p.Key, 1, int64(Percent))
	if err != nil || n == 0 {
		return err
	}
	*p.Target = Watermark(n)
	return nil
}

// number parses key in [lo, hi]; unset returns 0.
func (l SettingsLoader) number(key config.EnvKey, lo, hi int64) (int64, error) {
	v, ok := l.Lookup(string(key))
	if !ok || v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("membudget: %s must be an integer in [%d, %d], got %q", key, lo, hi, v)
	}
	return n, nil
}
