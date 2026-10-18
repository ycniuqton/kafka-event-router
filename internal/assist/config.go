// Package assist is the Transform Studio's assistant: the provider-agnostic LLMProvider family,
// step proposals from an instruction, explanations of steps and the recipe → JS exporter.
package assist

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/eventrouter/router/internal/assist/infer"
	"github.com/eventrouter/router/internal/platform/config"
)

// Env keys of the assistant (read by cmd/api only).
const (
	EnvAssistProvider  config.EnvKey = "ROUTER_ASSIST_PROVIDER"
	EnvAssistTimeout   config.EnvKey = "ROUTER_ASSIST_TIMEOUT"
	EnvAssistRateLimit config.EnvKey = "ROUTER_ASSIST_RATE_PER_MINUTE"
	// EnvAssistInferMatchers orders the infer-mapping matchers (comma-separated: value,name,type).
	EnvAssistInferMatchers config.EnvKey = "ROUTER_ASSIST_INFER_MATCHERS"
)

// SettingDefault holds textual defaults of the assistant settings.
type SettingDefault string

const DefaultProvider SettingDefault = SettingDefault(ProviderMock)

// SettingNumber holds numeric defaults of the assistant settings.
type SettingNumber int

const DefaultRatePerMinute SettingNumber = 30

// SettingDuration holds duration defaults of the assistant settings.
type SettingDuration time.Duration

const DefaultTimeout SettingDuration = SettingDuration(20 * time.Second)

// Settings is the assistant configuration: which provider answers, how requests are bounded and the
// infer-mapping matcher order.
type Settings struct {
	Provider      ProviderKind
	Timeout       time.Duration
	RatePerMinute int
	Limits        Limits
	InferMatchers []infer.Reason
}

// SettingsLoader reads Settings from the environment.
type SettingsLoader struct {
	Lookup config.LookupFunc
}

// NewSettingsLoader reads from the process environment.
func NewSettingsLoader() SettingsLoader {
	return SettingsLoader{Lookup: os.LookupEnv}
}

// Load builds the settings; malformed numbers or durations are errors.
func (l SettingsLoader) Load() (Settings, error) {
	timeout, err := l.duration(EnvAssistTimeout, time.Duration(DefaultTimeout))
	if err != nil {
		return Settings{}, err
	}
	rate, err := l.number(EnvAssistRateLimit, DefaultRatePerMinute)
	if err != nil {
		return Settings{}, err
	}
	limits := DefaultLimits()
	limits.Timeout = timeout
	return Settings{Provider: ProviderKind(l.str(EnvAssistProvider, DefaultProvider)), Timeout: timeout, RatePerMinute: rate, Limits: limits,
		InferMatchers: l.matchers()}, nil
}

// matchers reads the comma-separated matcher order (default value, name, type).
func (l SettingsLoader) matchers() []infer.Reason {
	v, ok := l.Lookup(string(EnvAssistInferMatchers))
	if !ok || strings.TrimSpace(v) == "" {
		return infer.DefaultOrder
	}
	out := []infer.Reason{}
	for _, part := range strings.Split(v, string(MatcherSeparator)) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, infer.Reason(part))
		}
	}
	return out
}

// ListSeparator splits list-valued settings.
type ListSeparator string

const MatcherSeparator ListSeparator = ","

func (l SettingsLoader) str(key config.EnvKey, def SettingDefault) string {
	if v, ok := l.Lookup(string(key)); ok && v != "" {
		return v
	}
	return string(def)
}

func (l SettingsLoader) number(key config.EnvKey, def SettingNumber) (int, error) {
	v, ok := l.Lookup(string(key))
	if !ok || v == "" {
		return int(def), nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("assist: %s must be a positive integer, got %q", key, v)
	}
	return n, nil
}

func (l SettingsLoader) duration(key config.EnvKey, def time.Duration) (time.Duration, error) {
	v, ok := l.Lookup(string(key))
	if !ok || v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("assist: %s must be a positive duration, got %q", key, v)
	}
	return d, nil
}
