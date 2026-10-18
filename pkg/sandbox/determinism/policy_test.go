package determinism

import (
	"strings"
	"testing"
	"time"
)

func TestSeedPolicyStable(t *testing.T) {
	p := SeedPolicy{}
	a := p.Seed("route-1", "orders", 3, 1234)
	if a != p.Seed("route-1", "orders", 3, 1234) {
		t.Fatal("seed must be stable")
	}
	if got := p.Seed("route-1", "orders", 3, 1234); got != pinnedSeed {
		t.Fatalf("seed changed to %#x; bump VersionCurrent if intended", got)
	}
	for _, other := range []uint64{p.Seed("route-2", "orders", 3, 1234), p.Seed("route-1", "order", 3, 1234),
		p.Seed("route-1", "orders", 4, 1234), p.Seed("route-1", "orders", 3, 1235), p.Seed("route-1o", "rders", 3, 1234)} {
		if other == a {
			t.Fatal("seed must depend on route, topic, partition and offset")
		}
	}
}

// pinnedSeed is the golden seed of ("route-1", "orders", 3, 1234) under VersionCurrent.
const pinnedSeed uint64 = 0x9d2d350164875eea

func TestRandomStateMatchesPreludeAlgorithm(t *testing.T) {
	s := SeedPolicy{}.State(42)
	first := []uint32{s.Next(), s.Next(), s.Next()}
	s2 := SeedPolicy{}.State(42)
	for i, want := range first {
		if got := s2.Next(); got != want {
			t.Fatalf("output %d = %d, want %d", i, got, want)
		}
	}
	if (SeedPolicy{}).State(43) == (SeedPolicy{}).State(42) {
		t.Fatal("different seeds must give different states")
	}
}

func TestFrozenClock(t *testing.T) {
	c := Freeze(time.Date(2024, 1, 1, 0, 0, 0, 123456789, time.FixedZone("x", 3600)))
	if c.Now().Location() != time.UTC || c.Now().Nanosecond() != 123000000 {
		t.Fatalf("frozen = %v", c.Now())
	}
	if c.UnixMilli() != c.Now().UnixMilli() || c.UnixNano() != c.Now().UnixNano() {
		t.Fatal("clock views disagree")
	}
}

func TestPreludeEmbedded(t *testing.T) {
	p := DefaultPrelude()
	for _, frag := range []string{"return [invoke, describeJSON, lockdown];", "'use strict'", "Math, 'random'"} {
		if !strings.Contains(p.Source, frag) {
			t.Errorf("prelude misses %q", frag)
		}
	}
}
