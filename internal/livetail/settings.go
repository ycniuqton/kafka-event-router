package livetail

import (
	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/platform/config"
)

// Configuration keys of the live tail.
const (
	EnvLiveTailTap  config.EnvKey = "ROUTER_LIVETAIL_TAP"
	EnvLiveTailRate config.EnvKey = "ROUTER_LIVETAIL_RATE"
)

// Settings selects the worker tap and its per-route rate.
type Settings struct {
	Tap    TapKind
	Limits TailLimits
}

// SettingsLoader reads Settings (defaults: redis tap, 20 records/s per route).
type SettingsLoader struct {
	Lookup config.LookupFunc
}

// Load returns the settings; a non-positive rate keeps the default.
func (l SettingsLoader) Load() (Settings, error) {
	r := &metrics.EnvReader{Lookup: l.Lookup}
	limits := DefaultTailLimits()
	if rate := r.Int(EnvLiveTailRate, int64(limits.RatePerSec)); rate > 0 {
		limits.RatePerSec, limits.Burst = int(rate), int(rate)
	}
	return Settings{Tap: TapKind(r.Str(EnvLiveTailTap, string(TapRedis))), Limits: limits}, r.Err()
}

// LogAttr names the live tail's structured log attributes.
type LogAttr string

const (
	AttrTap   LogAttr = "tap"
	AttrRoute LogAttr = "route_id"
)
