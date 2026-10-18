package main

import (
	"sync"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/sandboxinfo"
	"github.com/eventrouter/router/pkg/sandbox"
)

// sandboxWiring composes the js-sandbox endpoints once per process: the Studio runtime info, the
// editor compile check (engine booted on first use) and the per-team hash salt for workloads.
type sandboxWiring struct {
	once     sync.Once
	public   api.Module
	internal api.Module
}

var sandboxFeature = &sandboxWiring{}

// sandboxModule returns the public /api/v1/sandbox module.
func (s *apiService) sandboxModule() api.Module {
	sandboxFeature.build(s.wiring)
	return sandboxFeature.public
}

// sandboxInternalModule returns the workload-only /internal/v1/sandbox module.
func (s *apiService) sandboxInternalModule() api.Module {
	sandboxFeature.build(s.wiring)
	return sandboxFeature.internal
}

func (c *sandboxWiring) build(w *Wiring) {
	c.once.Do(func() {
		log := w.Deps.Log
		cfg, err := sandboxinfo.NewSettingsLoader().Load()
		if err != nil {
			log.Error(string(sandboxinfo.LogSettingsInvalid), string(logging.AttrError), err)
			cfg = sandbox.DefaultConfig()
		}
		d := sandboxinfo.Deps{
			Info:      sandboxinfo.StaticInfo{Config: cfg},
			Checker:   &sandboxinfo.LazyChecker{Config: cfg},
			MaxSource: sandbox.NewLimitPolicy(cfg.Maxima).Defaults.MaxSourceBytes,
			Salts:     sandboxinfo.NewTeamSalts(w.Deps.Secrets, w.Identity.Teams),
			Recorder:  w.Deps.Audit,
			Responder: w.Responder,
		}
		c.internal = sandboxinfo.NewInternalModule(d)
		c.public, err = sandboxinfo.NewModule(d)
		if err != nil {
			log.Error(string(sandboxinfo.LogModuleDisabled), string(logging.AttrError), err)
			c.public = api.Modules{}
		}
	})
}
