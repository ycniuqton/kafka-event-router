package metrics

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/eventrouter/router/internal/metrics/query"
	"github.com/eventrouter/router/internal/platform/config"
)

// Configuration keys of the monitoring feature (feature-owned, like routes and connections).
const (
	EnvMetricsBackend    config.EnvKey = "ROUTER_METRICS_BACKEND"
	EnvMetricsURL        config.EnvKey = "ROUTER_METRICS_URL"
	EnvMetricsTimeout    config.EnvKey = "ROUTER_METRICS_TIMEOUT"
	EnvRouteLagWarn      config.EnvKey = "ROUTER_MONITORING_ROUTE_LAG_WARN"
	EnvRouteLagError     config.EnvKey = "ROUTER_MONITORING_ROUTE_LAG_ERROR"
	EnvTotalLagWarn      config.EnvKey = "ROUTER_MONITORING_TOTAL_LAG_WARN"
	EnvTotalLagError     config.EnvKey = "ROUTER_MONITORING_TOTAL_LAG_ERROR"
	EnvZeroMatchWindow   config.EnvKey = "ROUTER_MONITORING_ZERO_MATCH_WINDOW"
	EnvLagGrowthWindow   config.EnvKey = "ROUTER_MONITORING_LAG_GROWTH_WINDOW"
	EnvAttentionDetector config.EnvKey = "ROUTER_MONITORING_ATTENTION_DETECTORS"
)

// AttentionKind names one "Needs attention" detector and the rows it produces.
type AttentionKind string

const (
	AttentionRouteError       AttentionKind = "route_error"
	AttentionLagGrowing       AttentionKind = "lag_growing"
	AttentionConnectionHealth AttentionKind = "connection_unhealthy"
	AttentionZeroMatch        AttentionKind = "zero_match"
	AttentionFiringAlert      AttentionKind = "alert_firing"
)

// DefaultAttentionKinds are the detectors enabled when the config names none (firing alerts since P2).
var DefaultAttentionKinds = []AttentionKind{AttentionRouteError, AttentionLagGrowing, AttentionConnectionHealth, AttentionZeroMatch,
	AttentionFiringAlert}

// SettingDefaults are the fallback connection values of the metrics store.
type SettingDefaults string

const (
	DefaultMetricsURL     SettingDefaults = "http://localhost:9090"
	DefaultMetricsTimeout SettingDefaults = "2s"
)

// Settings is the monitoring configuration read from the environment.
type Settings struct {
	Backend    query.BackendKind
	URL        string
	Timeout    time.Duration
	Thresholds Thresholds
	Detectors  []AttentionKind
}

// SettingsLoader reads Settings through a lookup function (os.LookupEnv in production).
type SettingsLoader struct {
	Lookup config.LookupFunc
}

// Load returns the settings with documented defaults, rejecting malformed values.
func (l SettingsLoader) Load() (Settings, error) {
	r := &EnvReader{Lookup: l.Lookup}
	s := Settings{
		Backend:   query.BackendKind(r.Str(EnvMetricsBackend, string(query.BackendPrometheus))),
		URL:       r.Str(EnvMetricsURL, string(DefaultMetricsURL)),
		Timeout:   r.Duration(EnvMetricsTimeout, DefaultMetricsTimeout.Duration()),
		Detectors: r.detectors(),
	}
	d := DefaultThresholds
	s.Thresholds = Thresholds{
		RouteLagWarn: r.Int(EnvRouteLagWarn, d.RouteLagWarn), RouteLagError: r.Int(EnvRouteLagError, d.RouteLagError),
		TotalLagWarn: r.Int(EnvTotalLagWarn, d.TotalLagWarn), TotalLagError: r.Int(EnvTotalLagError, d.TotalLagError),
		ZeroMatchWindow: r.Duration(EnvZeroMatchWindow, d.ZeroMatchWindow),
		LagGrowthWindow: r.Duration(EnvLagGrowthWindow, d.LagGrowthWindow),
	}
	return s, r.err
}

// Duration parses a duration default (defaults are valid by construction).
func (d SettingDefaults) Duration() time.Duration {
	v, _ := time.ParseDuration(string(d))
	return v
}

// EnvReader reads typed settings and keeps the first parse error (shared by the monitoring folder's packages).
type EnvReader struct {
	Lookup config.LookupFunc
	err    error
}

// Err returns the first parse error.
func (r *EnvReader) Err() error {
	return r.err
}

// Bool reads true/false.
func (r *EnvReader) Bool(key config.EnvKey, fallback bool) bool {
	raw := r.Str(key, "")
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		r.fail(fmt.Errorf("%s: invalid boolean %q", key, raw))
		return fallback
	}
	return v
}

// List reads a comma-separated list.
func (r *EnvReader) List(key config.EnvKey) []string {
	var out []string
	for _, part := range strings.Split(r.Str(key, ""), string(config.CommaSeparator)) {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (r *EnvReader) Str(key config.EnvKey, fallback string) string {
	if v, ok := r.Lookup(string(key)); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func (r *EnvReader) Duration(key config.EnvKey, fallback time.Duration) time.Duration {
	raw := r.Str(key, "")
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		r.fail(fmt.Errorf("%s: invalid duration %q", key, raw))
		return fallback
	}
	return d
}

func (r *EnvReader) Int(key config.EnvKey, fallback int64) int64 {
	raw := r.Str(key, "")
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		r.fail(fmt.Errorf("%s: invalid number %q", key, raw))
		return fallback
	}
	return n
}

func (r *EnvReader) detectors() []AttentionKind {
	raw := r.Str(EnvAttentionDetector, "")
	if raw == "" {
		return DefaultAttentionKinds
	}
	var out []AttentionKind
	for _, part := range strings.Split(raw, string(config.CommaSeparator)) {
		if k := AttentionKind(strings.TrimSpace(part)); k != "" {
			out = append(out, k)
		}
	}
	return out
}

func (r *EnvReader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}
