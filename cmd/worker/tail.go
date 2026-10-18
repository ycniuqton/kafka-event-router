package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"

	"github.com/eventrouter/router/internal/livetail"
	"github.com/eventrouter/router/internal/livetail/workertap"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/runtime/runner"
)

// TailLog names the worker's live-tail log messages.
type TailLog string

const (
	LogTailDisabled TailLog = "worker: live tail disabled"
	LogTailReady    TailLog = "worker: live tail tap ready"
)

// tailTap is the live-tail tap every runner offers finished records to (route-runtime RR-205 call site):
// monitoring's TapManager selects it from ROUTER_LIVETAIL_TAP; Redis is optional, so no readiness check.
func (a Assembly) tailTap() runner.TailTap {
	log := a.Deps.Log
	settings, err := livetail.SettingsLoader{Lookup: os.LookupEnv}.Load()
	if err != nil {
		log.Error(string(LogTailDisabled), string(logging.AttrError), err)
		return runner.NoopTail{}
	}
	client := a.tailRedis()
	drops, err := livetail.NewTapMetrics(a.Registry.Registerer())
	if err != nil {
		log.Error(string(LogTailDisabled), string(logging.AttrError), err)
		return runner.NoopTail{}
	}
	tap, err := livetail.NewTapManager(client != nil).Select(settings.Tap,
		livetail.TapDeps{Redis: client, Limits: settings.Limits, Drops: drops, Log: log})
	if err != nil {
		log.Error(string(LogTailDisabled), string(logging.AttrError), err)
		return runner.NoopTail{}
	}
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go tap.Run(ctx)
	log.Info(string(LogTailReady), string(livetail.AttrTap), string(settings.Tap))
	return workertap.RunnerTap{Tap: tap}
}

// tailRedis opens the worker's own Redis client for the tail (nil when the URL is invalid).
func (a Assembly) tailRedis() redis.UniversalClient {
	opts, err := redis.ParseURL(a.Deps.Config.Redis.URL)
	if err != nil {
		a.Deps.Log.Warn(string(LogTailDisabled), string(logging.AttrError), err)
		return nil
	}
	return redis.NewClient(opts)
}
