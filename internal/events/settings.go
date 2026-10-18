package events

import (
	"github.com/eventrouter/router/internal/metrics"
	"github.com/eventrouter/router/internal/platform/config"
)

// Configuration keys of push events.
const (
	EnvEventsBus        config.EnvKey = "ROUTER_EVENTS_BUS"
	EnvEventsVisibility config.EnvKey = "ROUTER_EVENTS_VISIBILITY"
)

// Settings selects the bus and the visibility policy.
type Settings struct {
	Bus        BusKind
	Visibility VisibilityKind
}

// SettingsLoader reads Settings (defaults: redis bus, open visibility).
type SettingsLoader struct {
	Lookup config.LookupFunc
}

// Load returns the settings.
func (l SettingsLoader) Load() Settings {
	r := &metrics.EnvReader{Lookup: l.Lookup}
	return Settings{Bus: BusKind(r.Str(EnvEventsBus, string(BusRedis))),
		Visibility: VisibilityKind(r.Str(EnvEventsVisibility, string(VisibilityOpen)))}
}
