package membudget

import (
	"log/slog"
	"os"

	"github.com/prometheus/client_golang/prometheus"
)

// LogAttr names the start-up log attributes of the budget.
type LogAttr string

const (
	AttrBudgetBytes LogAttr = "budget_bytes"
	AttrMemoryLimit LogAttr = "memory_limit_bytes"
	AttrLimitSource LogAttr = "limit_source"
	AttrGoMemLimit  LogAttr = "gomemlimit_bytes"
	AttrGoMemSet    LogAttr = "gomemlimit_applied"
)

// RootDir is the filesystem root the cgroup files are read from.
type RootDir string

const HostRoot RootDir = "/"

// Builder sizes the worker budget from the environment, applies GOMEMLIMIT and registers the gauges.
type Builder struct {
	Loader  SettingsLoader
	Cgroup  CgroupLimits
	Runtime GoMemLimit
	Log     *slog.Logger
}

// NewBuilder reads the process environment, the host cgroup files and the Go runtime.
func NewBuilder(log *slog.Logger) Builder {
	return Builder{Loader: NewSettingsLoader(), Cgroup: CgroupLimits{FS: os.DirFS(string(HostRoot))}, Runtime: RuntimeMemLimit{}, Log: log}
}

// Build returns the sized budget with its gauges registered on reg.
func (b Builder) Build(reg prometheus.Registerer, worker string) (*Budget, error) {
	settings, err := b.Loader.Load()
	if err != nil {
		return nil, err
	}
	res := Resolver{Settings: settings, GoMemLimitEnv: b.Loader.GoMemLimitSet(), Cgroup: b.Cgroup, Runtime: b.Runtime}.Resolve()
	budget := New(res.Budget)
	if err := budget.Register(reg, worker); err != nil {
		return nil, err
	}
	b.Log.Info("worker: memory budget ready", string(AttrBudgetBytes), res.Budget, string(AttrMemoryLimit), res.MemoryLimit,
		string(AttrLimitSource), string(res.Source), string(AttrGoMemLimit), res.GoMemLimit, string(AttrGoMemSet), res.GoMemSet)
	return budget, nil
}
