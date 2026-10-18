package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/internal/library/resolver"
	"github.com/eventrouter/router/internal/platform/app"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/platform/controlplane"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/platform/metrics"
	"github.com/eventrouter/router/internal/runtime/cache"
	"github.com/eventrouter/router/internal/runtime/clients"
	"github.com/eventrouter/router/internal/runtime/cpclient"
	"github.com/eventrouter/router/internal/runtime/delivery"
	"github.com/eventrouter/router/internal/runtime/domain"
	"github.com/eventrouter/router/internal/runtime/failures"
	"github.com/eventrouter/router/internal/runtime/producers"
	"github.com/eventrouter/router/internal/runtime/reconcile"
	"github.com/eventrouter/router/internal/runtime/retry"
	"github.com/eventrouter/router/internal/runtime/runner"
	"github.com/eventrouter/router/internal/sandboxinfo"
	pkgconfig "github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/compile"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/kafka"
	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/enginebind"
	"github.com/eventrouter/router/pkg/sandbox/engines"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// Assembly composes the worker runtime from env settings and shared infrastructure.
type Assembly struct {
	Deps     *app.Deps
	Registry *metrics.Registry
}

// Built is the composed runtime.
type Built struct {
	Reconciler *reconcile.Reconciler
}

// Build wires control-plane client, cache, runner manager and reconciler.
func (a Assembly) Build(ctx context.Context) (Built, error) {
	wc, err := config.NewRuntimeLoader().Worker()
	if err != nil {
		return Built{}, err
	}
	cp, err := controlplane.NewClient(a.Deps.Config.ControlPlane, a.Deps.Config.Internal.WorkloadAuth)
	if err != nil {
		return Built{}, fmt.Errorf("worker: control plane client: %w", err)
	}
	store, err := a.cache(wc.CacheDir)
	if err != nil {
		return Built{}, err
	}
	api := cpclient.New(cp, wc.ID)
	dlq := failures.DLQTargetResolver{Default: domain.DLQTargetKind(wc.DLQTarget)}
	halts := reconcile.NewHaltQueue()
	deps, err := a.runnerDeps(wc, dlq)
	if err != nil {
		return Built{}, err
	}
	runners := runner.NewManager(runner.RouteRunnerFactory{Deps: deps}, halts, a.Deps.Log)
	salts, err := sandboxinfo.NewSaltSourceRegistry(sandboxinfo.NewControlPlaneSalts(cp), reconcile.NewProcessSalts()).
		Build(sandboxinfo.NewSettingsLoader().SaltSource())
	if err != nil {
		return Built{}, err
	}
	library := resolver.NewControlPlaneResolver(cp)
	compiler := reconcile.EngineVersionCompiler{Engine: compile.NewDefaultCompiler(), Specs: reconcile.NewSpecVault(api),
		Scripts: a.scripts(ctx, salts, library), Salts: salts, DLQ: dlq, Library: library}
	rec := reconcile.New(reconcile.Deps{ControlPlane: api, Cache: store, Runners: runners, Compiler: compiler,
		Reload: reconcile.ConfigReloadClassifier{DLQ: dlq}, Halts: halts, Timings: domain.DefaultTimings(), Retry: retry.DefaultPolicy(),
		Identity: domain.WorkerIdentity{ID: wc.ID, Host: wc.ID, Zone: wc.Zone, Version: wc.Version, Capacity: wc.Capacity},
		Metrics:  reconcile.NewMetrics(a.Registry.Registerer()), Log: a.Deps.Log})
	halts.OnHalt(rec.Trigger)
	return Built{Reconciler: rec}, nil
}

// cache opens the configured cache directory, falling back to a temp directory when it is not writable.
func (a Assembly) cache(dir string) (*cache.FileConfigCache, error) {
	c, err := cache.NewFileConfigCache(dir)
	if err == nil {
		return c, nil
	}
	fallback := filepath.Join(os.TempDir(), string(CacheFallbackDir))
	a.Deps.Log.Warn("worker: cache dir not writable; using temp dir", string(logging.AttrError), err, string(logging.AttrPath), fallback)
	return cache.NewFileConfigCache(fallback)
}

// CacheDirName names the fallback cache directory under the temp dir.
type CacheDirName string

const CacheFallbackDir CacheDirName = "router-worker-cache"

// runnerDeps builds the worker-wide collaborators of every runner (producer sharing from env config).
func (a Assembly) runnerDeps(wc config.WorkerConfig, dlq failures.DLQTargetResolver) (runner.Deps, error) {
	sharing := domain.ProducerSharing(wc.ProducerSharing)
	if !sharing.Valid() {
		return runner.Deps{}, fmt.Errorf("worker: %s: unknown producer sharing %q", config.EnvWorkerProducerSharing, wc.ProducerSharing)
	}
	opener := clients.NewOpenerManager(clients.NewFranzOpener(), kafka.DefaultDemoClientFactory())
	policy := retry.DefaultPolicy()
	pool := producers.NewPool(opener, sharing, policy)
	if err := pool.Register(a.Registry.Registerer()); err != nil {
		return runner.Deps{}, err
	}
	a.Deps.Log.Info("worker: producer pool ready", string(producers.AttrSharing), string(sharing))
	return runner.Deps{Opener: opener, Producers: pool,
		Delivery: delivery.NewDefaultManager(), Classifier: failures.DefaultClassifier{}, DLQTargets: dlq, Encoder: record.Codec{},
		Metrics: runner.NewMetrics(a.Registry.Registerer(), wc.ID), Timings: domain.DefaultTimings(), Retry: policy, Log: a.Deps.Log, Now: time.Now, Tail: a.tailTap()}, nil
}

// scripts starts the worker sandbox (ROUTER_SANDBOX_* settings, route JS metrics); without it JS steps
// fail to compile (reported per version). ctx.lookup reads pinned library tables through library.
func (a Assembly) scripts(ctx context.Context, salts host.SaltProvider, library pkgconfig.LibraryResolver) reconcile.ScriptProvider {
	cfg, err := sandboxinfo.NewSettingsLoader().Load()
	if err != nil {
		a.Deps.Log.Error(string(sandboxinfo.LogSettingsInvalid), string(logging.AttrError), err)
		cfg = sandbox.DefaultConfig()
	}
	var observer sandbox.Observer = sandbox.NopObserver{}
	prom, err := sandbox.NewPromObserver(a.Registry.Registerer(), sandbox.HostOwnedSeries...)
	if err != nil {
		a.Deps.Log.Error(string(sandboxinfo.LogMetricsOff), string(logging.AttrError), err)
	} else {
		observer = prom
	}
	m, err := sandbox.NewManager(ctx, cfg, sandbox.ManagerDeps{Engines: engines.Registry(), Salts: salts,
		Binder: enginebind.Binder{Base: ctxhelpers.Set{}}, Observer: observer, Library: enginebind.LibraryTables{Resolver: library}})
	if err != nil {
		a.Deps.Log.Error("worker: sandbox unavailable; js steps disabled", string(logging.AttrError), err)
		return reconcile.NoScripts{}
	}
	return reconcile.SandboxScripts{Manager: m}
}
