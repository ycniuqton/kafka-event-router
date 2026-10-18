// Command worker runs the data plane: route runners (consume, filter/transform, produce, DLQ, commits)
// reconciled against the control plane's leases, plus probes and /metrics.
package main

import (
	"context"
	"os"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/platform/app"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/httpserver"
	"github.com/eventrouter/router/internal/platform/metrics"
)

// workerService exposes the worker's operational endpoints and runs the reconciler until shutdown.
type workerService struct {
	registry *metrics.Registry
	running  sync.WaitGroup
}

func (s *workerService) Name() config.ServiceName { return config.ServiceWorker }

// BuildDeps implements app.DepsBuilder: it assembles the runtime and starts reconciling on ctx
// (cancelled by SIGINT/SIGTERM, which drains every runner).
func (s *workerService) BuildDeps(ctx context.Context, deps *app.Deps) error {
	built, err := Assembly{Deps: deps, Registry: s.registry}.Build(ctx)
	if err != nil {
		return err
	}
	s.running.Add(1)
	go func() {
		defer s.running.Done()
		built.Reconciler.Run(ctx)
	}()
	return nil
}

func (s *workerService) Mount(r chi.Router, _ app.Deps) {
	r.Handle(string(httpserver.PathMetrics), s.registry.Handler())
}

func main() {
	svc := &workerService{registry: metrics.NewRegistry()}
	code := app.NewRunner(svc).Run()
	svc.running.Wait()
	os.Exit(int(code))
}
