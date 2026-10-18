package sandbox

import (
	"sync"
	"time"
)

// RouteUsage is the sandbox footprint of one route version: live instances, their memory and
// the cumulative guest CPU time, summed over every pool of the version.
type RouteUsage struct {
	RouteID     string
	VersionID   string
	Modules     int
	Instances   int
	ColdStarts  uint64
	MemoryBytes int64
	Calls       uint64
	CPUTime     time.Duration
}

// Add folds one pool into the usage.
func (u RouteUsage) Add(st PoolStats) RouteUsage {
	u.Modules++
	u.Instances += st.Size
	u.ColdStarts += st.ColdStarts
	u.MemoryBytes += st.MemoryBytes
	u.Calls += st.Calls
	u.CPUTime += st.CPUTime
	return u
}

// UsageSource lists the usage of every open route sandbox (implemented by DefaultManager).
type UsageSource interface {
	Usage() []RouteUsage
}

// RouteLoad is the sampled load of one route version for worker capacity scoring: CPUShare is the
// guest CPU time per wall-clock second since the previous sample (1.0 = one busy core).
type RouteLoad struct {
	RouteID     string
	VersionID   string
	CPUShare    float64
	MemoryBytes int64
	Instances   int
}

// usageKey identifies a route version between samples.
type usageKey struct {
	route, version string
}

// UsageSampler turns cumulative usage into per-interval load; safe for concurrent use.
type UsageSampler struct {
	source UsageSource
	mu     sync.Mutex
	last   map[usageKey]time.Duration
	lastAt time.Time
}

// NewUsageSampler samples source; the first Sample reports CPU since each sandbox opened.
func NewUsageSampler(source UsageSource, start time.Time) *UsageSampler {
	return &UsageSampler{source: source, last: map[usageKey]time.Duration{}, lastAt: start}
}

// Sample returns the load of every open route version since the previous sample.
func (s *UsageSampler) Sample(now time.Time) []RouteLoad {
	s.mu.Lock()
	defer s.mu.Unlock()
	elapsed := now.Sub(s.lastAt)
	merged := UsageIndex{}.Merge(s.source.Usage())
	next := make(map[usageKey]time.Duration, len(merged))
	out := make([]RouteLoad, 0, len(merged))
	for key, u := range merged {
		load := RouteLoad{RouteID: u.RouteID, VersionID: u.VersionID, MemoryBytes: u.MemoryBytes, Instances: u.Instances}
		if elapsed > 0 {
			load.CPUShare = max(0, float64(u.CPUTime-s.last[key])/float64(elapsed))
		}
		next[key] = u.CPUTime
		out = append(out, load)
	}
	s.last, s.lastAt = next, now
	return out
}

// UsageIndex merges usage entries of the same route version (one per open sandbox).
type UsageIndex struct{}

// Merge sums usages by route version.
func (UsageIndex) Merge(list []RouteUsage) map[usageKey]RouteUsage {
	out := map[usageKey]RouteUsage{}
	for _, u := range list {
		key := usageKey{u.RouteID, u.VersionID}
		cur, ok := out[key]
		if !ok {
			out[key] = u
			continue
		}
		cur.Modules += u.Modules
		cur.Instances += u.Instances
		cur.MemoryBytes += u.MemoryBytes
		cur.Calls += u.Calls
		cur.CPUTime += u.CPUTime
		out[key] = cur
	}
	return out
}
