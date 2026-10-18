package metrics

import (
	"testing"
	"time"

	"github.com/eventrouter/router/internal/metrics/query"
)

func lookup(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
}

func TestSettingsDefaults(t *testing.T) {
	s, err := SettingsLoader{Lookup: lookup(nil)}.Load()
	if err != nil || s.Backend != query.BackendPrometheus || s.Timeout != 2*time.Second || s.Thresholds != DefaultThresholds ||
		len(s.Detectors) != len(DefaultAttentionKinds) {
		t.Fatalf("defaults %+v %v", s, err)
	}
}

func TestSettingsOverridesAndErrors(t *testing.T) {
	s, err := SettingsLoader{Lookup: lookup(map[string]string{string(EnvMetricsBackend): "mock", string(EnvRouteLagWarn): "5",
		string(EnvAttentionDetector): "route_error, zero_match", string(EnvZeroMatchWindow): "10m"})}.Load()
	if err != nil || s.Backend != query.BackendMock || s.Thresholds.RouteLagWarn != 5 || len(s.Detectors) != 2 ||
		s.Thresholds.ZeroMatchWindow != 10*time.Minute {
		t.Fatalf("overrides %+v %v", s, err)
	}
	if _, err := (SettingsLoader{Lookup: lookup(map[string]string{string(EnvMetricsTimeout): "soon"})}).Load(); err == nil {
		t.Fatal("bad duration accepted")
	}
}

func TestRangesAndTones(t *testing.T) {
	if r, err := ParseRange(""); err != nil || r != Range1h {
		t.Fatalf("default range %v %v", r, err)
	}
	if _, err := ParseRange("2w"); err == nil {
		t.Fatal("unknown range accepted")
	}
	if Range7d.Step() != time.Hour || Range15m.Duration() != 15*time.Minute {
		t.Fatal("range table")
	}
	th := DefaultThresholds
	if th.TotalTone(50000) != ToneErr || th.TotalTone(20000) != ToneWarn || th.RouteTone(100) != ToneOK {
		t.Fatal("tones")
	}
}
