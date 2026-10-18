package retry

import (
	"testing"
	"time"
)

func TestPolicyStaysWithinBoundsAndReachesCap(t *testing.T) {
	p := DefaultPolicy()
	for attempt := 0; attempt < 20; attempt++ {
		if d := p.Delay(attempt); d < p.Min || d > p.Max {
			t.Fatalf("attempt %d: %s outside [%s, %s]", attempt, d, p.Min, p.Max)
		}
	}
	if p.Delay(30) < time.Duration(float64(p.Max)*(1-p.Jitter)) {
		t.Fatal("late attempts must be near the cap")
	}
	if p.Delay(0) > time.Duration(float64(p.Min)*(1+p.Jitter)) {
		t.Fatal("first attempt must be near the minimum")
	}
	if p.BackoffFn()(1) > time.Duration(float64(p.Min)*(1+p.Jitter)) {
		t.Fatal("franz-go tries are 1-based")
	}
}
