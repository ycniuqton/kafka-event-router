package config

import (
	"fmt"
	"os"
	"strconv"
)

// Route-runtime environment keys (worker identity, DLQ default, scheduler).
const (
	EnvWorkerID              EnvKey = "ROUTER_WORKER_ID"
	EnvWorkerZone            EnvKey = "ROUTER_WORKER_ZONE"
	EnvWorkerCapacity        EnvKey = "ROUTER_WORKER_CAPACITY"
	EnvWorkerCacheDir        EnvKey = "ROUTER_WORKER_CACHE_DIR"
	EnvWorkerProducerSharing EnvKey = "ROUTER_WORKER_PRODUCER_SHARING"
	EnvDLQDefaultTarget      EnvKey = "ROUTER_DLQ_DEFAULT_TARGET"
	EnvSchedulerPlacement    EnvKey = "ROUTER_SCHEDULER_PLACEMENT"
	EnvSchedulerEnabled      EnvKey = "ROUTER_SCHEDULER_ENABLED"
	EnvWorkerBuildVersion    EnvKey = "ROUTER_WORKER_VERSION"
)

// RuntimeDefaults groups fallback values of the route-runtime settings.
type RuntimeDefaults string

const (
	DefaultWorkerZone       RuntimeDefaults = "default"
	DefaultWorkerCacheDir   RuntimeDefaults = "/var/lib/router/cache"
	DefaultDLQTarget        RuntimeDefaults = "source"
	DefaultPlacement        RuntimeDefaults = "least_loaded"
	DefaultProducerSharing  RuntimeDefaults = "on"
	DefaultWorkerVersion    RuntimeDefaults = "dev"
	DefaultWorkerIDFallback RuntimeDefaults = "worker"
)

// RuntimeNumbers groups numeric fallbacks of the route-runtime settings.
type RuntimeNumbers int

const DefaultWorkerCapacity RuntimeNumbers = 100

// WorkerConfig configures one data-plane worker process.
type WorkerConfig struct {
	ID              string
	Zone            string
	Capacity        int
	CacheDir        string
	DLQTarget       string
	ProducerSharing string
	Version         string
}

// SchedulerConfig configures the leader-only scheduler inside cmd/api.
type SchedulerConfig struct {
	Enabled   bool
	Placement string
}

// RuntimeLoader reads the route-runtime settings (kept apart from Config so the core loader stays stable).
type RuntimeLoader struct {
	lookup   LookupFunc
	hostname func() (string, error)
}

// NewRuntimeLoader reads from the process environment.
func NewRuntimeLoader() RuntimeLoader {
	return RuntimeLoader{lookup: os.LookupEnv, hostname: os.Hostname}
}

// NewRuntimeLoaderWithLookup reads from a custom lookup (tests).
func NewRuntimeLoaderWithLookup(lookup LookupFunc) RuntimeLoader {
	return RuntimeLoader{lookup: lookup, hostname: os.Hostname}
}

// Worker loads the worker settings; the id defaults to the hostname (stable pod name in k8s).
func (l RuntimeLoader) Worker() (WorkerConfig, error) {
	capacity, err := l.integer(EnvWorkerCapacity, int(DefaultWorkerCapacity))
	if err != nil {
		return WorkerConfig{}, err
	}
	return WorkerConfig{
		ID:              l.str(EnvWorkerID, l.host()),
		Zone:            l.str(EnvWorkerZone, string(DefaultWorkerZone)),
		Capacity:        capacity,
		CacheDir:        l.str(EnvWorkerCacheDir, string(DefaultWorkerCacheDir)),
		DLQTarget:       l.str(EnvDLQDefaultTarget, string(DefaultDLQTarget)),
		ProducerSharing: l.str(EnvWorkerProducerSharing, string(DefaultProducerSharing)),
		Version:         l.str(EnvWorkerBuildVersion, string(DefaultWorkerVersion)),
	}, nil
}

// Scheduler loads the scheduler settings.
func (l RuntimeLoader) Scheduler() (SchedulerConfig, error) {
	enabled := true
	if raw, ok := l.lookup(string(EnvSchedulerEnabled)); ok && raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return SchedulerConfig{}, fmt.Errorf("config: %s: %w", EnvSchedulerEnabled, err)
		}
		enabled = b
	}
	return SchedulerConfig{Enabled: enabled, Placement: l.str(EnvSchedulerPlacement, string(DefaultPlacement))}, nil
}

func (l RuntimeLoader) host() string {
	if h, err := l.hostname(); err == nil && h != "" {
		return h
	}
	return string(DefaultWorkerIDFallback)
}

func (l RuntimeLoader) str(key EnvKey, fallback string) string {
	if v, ok := l.lookup(string(key)); ok && v != "" {
		return v
	}
	return fallback
}

func (l RuntimeLoader) integer(key EnvKey, fallback int) (int, error) {
	raw, ok := l.lookup(string(key))
	if !ok || raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("config: %s: must be a non-negative integer", key)
	}
	return n, nil
}
