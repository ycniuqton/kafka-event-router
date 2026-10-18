// Command preview runs the preview/sample service: sample sessions, schema inference and
// traced preview runs on the shared engine, served internally under /internal/v1/preview.
package main

import (
	"context"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/app"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/internal/platform/metrics"
	"github.com/eventrouter/router/internal/preview"
)

// previewService exposes /metrics and the internal preview endpoints.
type previewService struct {
	registry *metrics.Registry
	settings preview.SettingsLoader
	built    Built
}

func (s *previewService) Name() config.ServiceName { return config.ServicePreview }

// Infra implements app.InfraConsumer: Redis when sessions live there, Postgres when test topics do.
func (s *previewService) Infra() []app.Infra {
	infra := []app.Infra{}
	if s.settings.Store() == preview.StoreRedis {
		infra = append(infra, app.InfraRedis)
	}
	if s.settings.TestTopicStore() == preview.TestTopicsPostgres {
		infra = append(infra, app.InfraPostgres)
	}
	return infra
}

// BuildDeps implements app.DepsBuilder.
func (s *previewService) BuildDeps(ctx context.Context, deps *app.Deps) error {
	settings, err := s.settings.Load(deps.Config.Environment)
	if err != nil {
		return err
	}
	s.built, err = Assembly{deps: deps, registry: s.registry, settings: settings, now: time.Now}.Build(ctx)
	return err
}

// Mount attaches /metrics and the preview module behind the directive stage.
func (s *previewService) Mount(r chi.Router, _ app.Deps) {
	r.Handle(string(httpserver.PathMetrics), s.registry.Handler())
	router := chi.NewRouter()
	s.built.Mounter.Install(router)
	s.built.Mounter.Mount(router, s.built.Module)
	r.Mount("/", router)
}

func main() {
	svc := &previewService{registry: metrics.NewRegistry(), settings: preview.NewSettingsLoader()}
	os.Exit(int(app.NewRunner(svc).Run()))
}
