package sandbox

import (
	"testing"
	"time"
)

func TestLimitPolicyResolve(t *testing.T) {
	policy := NewLimitPolicy(LimitMaximum)
	cases := []struct {
		name     string
		req      RouteLimits
		want     Limits
		warnings []LimitPath
	}{
		{name: "defaults", req: RouteLimits{}, want: LimitDefault},
		{name: "within maxima", req: RouteLimits{JSTimeoutMs: 200, JSMemoryMb: 128, JSMaxOutputKb: 2048},
			want: with(LimitDefault, 200*time.Millisecond, 128*int64(MiB), 2048*int64(KiB))},
		{name: "exactly at maxima", req: RouteLimits{JSTimeoutMs: 1000, JSMemoryMb: 256, JSMaxOutputKb: 8192},
			want: with(LimitDefault, time.Second, 256*int64(MiB), 8*int64(MiB))},
		{name: "above maxima clamps with warnings", req: RouteLimits{JSTimeoutMs: 5000, JSMemoryMb: 1024, JSMaxOutputKb: 99999},
			want:     with(LimitDefault, time.Second, 256*int64(MiB), 8*int64(MiB)),
			warnings: []LimitPath{PathJSTimeoutMs, PathJSMemoryMb, PathJSMaxOutputKb}},
		{name: "negative means default", req: RouteLimits{JSTimeoutMs: -1}, want: LimitDefault},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, issues := policy.Resolve(tc.req)
			if got != tc.want {
				t.Fatalf("limits = %+v, want %+v", got, tc.want)
			}
			if len(issues) != len(tc.warnings) {
				t.Fatalf("issues = %+v, want paths %v", issues, tc.warnings)
			}
			for i, d := range issues {
				if d.Severity != SeverityWarning || d.Path != string(tc.warnings[i]) {
					t.Errorf("issue %d = %+v", i, d)
				}
			}
		})
	}
}

func TestLimitPolicyOperatorCapBelowDefault(t *testing.T) {
	maxima := LimitMaximum
	maxima.MaxLogBytes = 1024
	got, _ := NewLimitPolicy(maxima).Resolve(RouteLimits{})
	if got.MaxLogBytes != 1024 {
		t.Fatalf("log cap = %d, want operator cap 1024", got.MaxLogBytes)
	}
}

func with(base Limits, timeout time.Duration, memory, output int64) Limits {
	base.Timeout, base.MemoryBytes, base.MaxOutputBytes = timeout, memory, output
	return base
}
