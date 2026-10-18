package pacing

import (
	"testing"
	"time"
)

func TestParseSteps(t *testing.T) {
	p, err := Planner{}.ParseSteps("1000:30s, 2500:1m")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 2 || p.Steps[1].Rate != 2500 || p.Total() != 90*time.Second {
		t.Errorf("plan = %+v", p)
	}
	for _, bad := range []string{"1000", "x:1s", "10:0s", "-1:1s"} {
		if _, err := (Planner{}).ParseSteps(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParseRampIsInclusive(t *testing.T) {
	p, err := Planner{}.ParseRamp("10000:50000:10000:5m")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 5 || p.Steps[4].Rate != 50000 || p.Steps[0].Duration != 5*time.Minute {
		t.Errorf("plan = %+v", p)
	}
	if _, err := (Planner{}).ParseRamp("1:2:3"); err == nil {
		t.Error("short ramp accepted")
	}
}

func TestPacerHoldsRate(t *testing.T) {
	start := time.Unix(0, 0)
	p := NewPacer(1000, start)
	var total int64
	for ms := 1; ms <= 2000; ms++ {
		total += p.Due(start.Add(time.Duration(ms) * time.Millisecond))
	}
	if total != 2000 {
		t.Errorf("released %d in 2s at 1000/s", total)
	}
}

func TestPacerCapsCatchUpBurst(t *testing.T) {
	start := time.Unix(0, 0)
	p := NewPacer(100000, start)
	if got := p.Due(start.Add(time.Second)); got != Limits.MaxBurst {
		t.Errorf("burst = %d", got)
	}
	if NewPacer(0, start).Due(start) != Limits.MaxBurst {
		t.Error("unlimited pacer must release a full burst")
	}
}
