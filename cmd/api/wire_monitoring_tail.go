package main

import (
	"os"

	"github.com/eventrouter/router/internal/livetail"
	livetailhttp "github.com/eventrouter/router/internal/livetail/http"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
)

// liveTailModule returns WS /routes/{id}/tail (P3): Redis tail store, access-control masking decision,
// live-preview's type-preserving masker and the livetail.opened audit entry.
func (s *apiService) liveTailModule() api.Module {
	w := s.wiring
	settings, err := livetail.SettingsLoader{Lookup: os.LookupEnv}.Load()
	if err != nil || w.Deps.Redis == nil {
		w.Deps.Log.Error(string(LogMonitoringDisabled), string(logging.AttrError), err)
		return api.Modules{}
	}
	var visibility livetail.VisibilityDecider = livetail.AlwaysMasked{}
	if w.Access != nil && w.Access.Samples != nil {
		visibility = livetail.RBACVisibility{Access: w.Access.Samples}
	}
	return livetailhttp.NewModule(w.WebSockets, livetailhttp.Deps{Routes: livetail.PgRouteDirectory{DB: w.Deps.DB},
		Store: livetail.RedisTailStore{Client: w.Deps.Redis, Limits: settings.Limits}, Visibility: visibility,
		Masker: livetail.NewTypePreservingTailMasker(), Audit: livetailhttp.RecorderAuditor{Recorder: w.Deps.Audit},
		Limits: settings.Limits, Log: w.Deps.Log})
}
