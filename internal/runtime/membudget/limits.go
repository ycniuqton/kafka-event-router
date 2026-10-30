package membudget

import (
	"errors"
	"io/fs"
	"math"
	"runtime/debug"
	"strconv"
	"strings"
)

// CgroupFile names the cgroup files holding the container memory limit (relative to the filesystem root).
type CgroupFile string

const (
	CgroupV2MemoryMax   CgroupFile = "sys/fs/cgroup/memory.max"
	CgroupV1MemoryLimit CgroupFile = "sys/fs/cgroup/memory/memory.limit_in_bytes"
)

// CgroupValue names special cgroup values.
type CgroupValue string

const CgroupUnlimited CgroupValue = "max"

// LimitBound names the bounds of a believable memory limit.
type LimitBound int64

const (
	CgroupUnlimitedAbove LimitBound = 1 << 60
	FallbackMemoryLimit  LimitBound = 1 << 30
)

// LimitSource names where the memory limit came from (logged at start-up).
type LimitSource string

const (
	SourceConfigured LimitSource = "configured"
	SourceGoMemLimit LimitSource = "gomemlimit"
	SourceCgroup     LimitSource = "cgroup"
	SourceFallback   LimitSource = "fallback"
)

// ErrNoCgroupLimit is returned when no cgroup memory limit is set.
var ErrNoCgroupLimit = errors.New("membudget: no cgroup memory limit")

// CgroupLimits reads the container memory limit from cgroup v2, then v1.
type CgroupLimits struct {
	FS fs.FS
}

// Limit returns the cgroup memory limit in bytes, or ErrNoCgroupLimit.
func (c CgroupLimits) Limit() (int64, error) {
	for _, f := range []CgroupFile{CgroupV2MemoryMax, CgroupV1MemoryLimit} {
		raw, err := fs.ReadFile(c.FS, string(f))
		if err != nil {
			continue
		}
		v := strings.TrimSpace(string(raw))
		if v == string(CgroupUnlimited) {
			return 0, ErrNoCgroupLimit
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err == nil && n > 0 && n < int64(CgroupUnlimitedAbove) {
			return n, nil
		}
	}
	return 0, ErrNoCgroupLimit
}

// GoMemLimit reads and sets the Go runtime soft memory limit (GOMEMLIMIT).
type GoMemLimit interface {
	Current() int64
	Set(limit int64)
}

// RuntimeMemLimit is GoMemLimit over runtime/debug.
type RuntimeMemLimit struct{}

// Current returns the soft limit, math.MaxInt64 when none is set.
func (RuntimeMemLimit) Current() int64 { return debug.SetMemoryLimit(-1) }

// Set sets the soft limit.
func (RuntimeMemLimit) Set(limit int64) { debug.SetMemoryLimit(limit) }

// Resolution is the outcome of sizing: the memory limit the budget derives from and the applied GOMEMLIMIT.
type Resolution struct {
	MemoryLimit int64
	Source      LimitSource
	Budget      int64
	GoMemLimit  int64
	GoMemSet    bool
}

// Resolver turns settings plus the environment (GOMEMLIMIT env, cgroup) into a budget size.
type Resolver struct {
	Settings      Settings
	GoMemLimitEnv bool
	Cgroup        CgroupLimits
	Runtime       GoMemLimit
}

// Resolve applies GOMEMLIMIT from the cgroup when unset, then sizes the budget.
func (r Resolver) Resolve() Resolution {
	cgroup, cgErr := r.Cgroup.Limit()
	res := Resolution{GoMemLimit: r.Runtime.Current()}
	if !r.GoMemLimitEnv && cgErr == nil {
		res.GoMemLimit = Watermark(r.Settings.GoMemLimitPercent).of(cgroup)
		r.Runtime.Set(res.GoMemLimit)
		res.GoMemSet = true
	}
	res.MemoryLimit, res.Source = r.memoryLimit(res.GoMemLimit, cgroup, cgErr)
	res.Budget = Watermark(r.Settings.BudgetPercent).of(res.MemoryLimit)
	if r.Settings.BudgetBytes > 0 {
		res.Budget, res.Source = r.Settings.BudgetBytes, SourceConfigured
	}
	return res
}

// memoryLimit picks GOMEMLIMIT set by the operator, else the cgroup limit, else the fallback.
func (r Resolver) memoryLimit(goLimit, cgroup int64, cgErr error) (int64, LimitSource) {
	switch {
	case r.GoMemLimitEnv && goLimit > 0 && goLimit < math.MaxInt64:
		return goLimit, SourceGoMemLimit
	case cgErr == nil:
		return cgroup, SourceCgroup
	}
	return int64(FallbackMemoryLimit), SourceFallback
}
