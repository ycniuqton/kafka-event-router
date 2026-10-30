package membudget

import (
	"errors"
	"math"
	"testing"
	"testing/fstest"
)

// fakeRuntime is a GoMemLimit that records Set.
type fakeRuntime struct {
	current int64
	set     int64
}

func (f *fakeRuntime) Current() int64  { return f.current }
func (f *fakeRuntime) Set(limit int64) { f.set, f.current = limit, limit }

func cgroupFS(files map[CgroupFile]string) CgroupLimits {
	m := fstest.MapFS{}
	for name, body := range files {
		m[string(name)] = &fstest.MapFile{Data: []byte(body)}
	}
	return CgroupLimits{FS: m}
}

func TestCgroupLimitsReadV2ThenV1(t *testing.T) {
	cases := []struct {
		name  string
		files map[CgroupFile]string
		want  int64
		err   error
	}{
		{"v2", map[CgroupFile]string{CgroupV2MemoryMax: "536870912\n"}, 536870912, nil},
		{"v2 unlimited", map[CgroupFile]string{CgroupV2MemoryMax: "max\n"}, 0, ErrNoCgroupLimit},
		{"v1", map[CgroupFile]string{CgroupV1MemoryLimit: "268435456"}, 268435456, nil},
		{"v1 unlimited", map[CgroupFile]string{CgroupV1MemoryLimit: "9223372036854771712"}, 0, ErrNoCgroupLimit},
		{"none", map[CgroupFile]string{}, 0, ErrNoCgroupLimit},
	}
	for _, c := range cases {
		got, err := cgroupFS(c.files).Limit()
		if got != c.want || !errors.Is(err, c.err) {
			t.Fatalf("%s: limit=%d err=%v, want %d %v", c.name, got, err, c.want, c.err)
		}
	}
}

func TestResolverAppliesGoMemLimitFromCgroupAndSizesBudget(t *testing.T) {
	rt := &fakeRuntime{current: math.MaxInt64}
	settings := Settings{BudgetPercent: DefaultBudgetPercent, GoMemLimitPercent: DefaultGoMemLimitPercent}
	res := Resolver{Settings: settings, Cgroup: cgroupFS(map[CgroupFile]string{CgroupV2MemoryMax: "1000000000"}), Runtime: rt}.Resolve()
	if !res.GoMemSet || rt.set != 900000000 {
		t.Fatalf("GOMEMLIMIT must be 90%% of the cgroup limit, set=%d", rt.set)
	}
	if res.Source != SourceCgroup || res.MemoryLimit != 1000000000 || res.Budget != 600000000 {
		t.Fatalf("resolution = %+v", res)
	}
}

func TestResolverKeepsOperatorGoMemLimit(t *testing.T) {
	rt := &fakeRuntime{current: 400 << 20}
	settings := Settings{BudgetPercent: 50, GoMemLimitPercent: DefaultGoMemLimitPercent}
	res := Resolver{Settings: settings, GoMemLimitEnv: true, Cgroup: cgroupFS(map[CgroupFile]string{CgroupV2MemoryMax: "1000000000"}), Runtime: rt}.Resolve()
	if res.GoMemSet || rt.set != 0 || res.Source != SourceGoMemLimit || res.Budget != 200<<20 {
		t.Fatalf("resolution = %+v set=%d", res, rt.set)
	}
}

func TestResolverFallbackAndConfiguredBytes(t *testing.T) {
	rt := &fakeRuntime{current: math.MaxInt64}
	settings := Settings{BudgetPercent: DefaultBudgetPercent, GoMemLimitPercent: DefaultGoMemLimitPercent}
	res := Resolver{Settings: settings, Cgroup: cgroupFS(nil), Runtime: rt}.Resolve()
	if res.Source != SourceFallback || res.GoMemSet || res.Budget != Watermark(DefaultBudgetPercent).of(int64(FallbackMemoryLimit)) {
		t.Fatalf("fallback resolution = %+v", res)
	}
	settings.BudgetBytes = 12345
	if res := (Resolver{Settings: settings, Cgroup: cgroupFS(nil), Runtime: rt}).Resolve(); res.Budget != 12345 || res.Source != SourceConfigured {
		t.Fatalf("configured resolution = %+v", res)
	}
}

func TestSettingsLoader(t *testing.T) {
	env := map[string]string{}
	l := SettingsLoader{Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok }}
	s, err := l.Load()
	if err != nil || s.BudgetPercent != DefaultBudgetPercent || s.GoMemLimitPercent != DefaultGoMemLimitPercent || s.BudgetBytes != 0 || l.GoMemLimitSet() {
		t.Fatalf("defaults = %+v, %v", s, err)
	}
	env[string(EnvBudgetBytes)], env[string(EnvBudgetPercent)], env[string(EnvGoMemLimit)] = "1048576", "40", "512MiB"
	if s, err = l.Load(); err != nil || s.BudgetBytes != 1<<20 || s.BudgetPercent != 40 || !l.GoMemLimitSet() {
		t.Fatalf("configured = %+v, %v", s, err)
	}
	for _, bad := range []struct{ key, value string }{{string(EnvBudgetPercent), "0"}, {string(EnvGoMemLimitPercent), "101"}, {string(EnvBudgetBytes), "-1"}, {string(EnvBudgetBytes), "x"}} {
		env[bad.key] = bad.value
		if _, err := l.Load(); err == nil {
			t.Fatalf("%s=%s must be rejected", bad.key, bad.value)
		}
		delete(env, bad.key)
	}
}
