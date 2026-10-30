package config_test

import (
	"testing"

	"github.com/eventrouter/router/pkg/config"
)

func intp(v int) *int { return &v }

func modep(m config.InFlightMode) *config.InFlightMode { return &m }

func TestInFlightBoundsDefaultsToAuto(t *testing.T) {
	var none *config.Limits
	got := none.InFlight()
	want := config.InFlightBounds{Mode: config.InFlightModeAuto, Min: 64, Max: 10000, Start: 1000}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestInFlightBoundsMaxIsCapInAuto(t *testing.T) {
	got := (&config.Limits{MaxInFlight: intp(500), MinInFlight: intp(10)}).InFlight()
	want := config.InFlightBounds{Mode: config.InFlightModeAuto, Min: 10, Max: 500, Start: 500}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
	got = (&config.Limits{MaxInFlight: intp(5000), MinInFlight: intp(2000)}).InFlight()
	if got.Start != 2000 || got.Max != 5000 {
		t.Fatalf("start must clamp into [floor, cap]: %+v", got)
	}
}

func TestInFlightBoundsFixedKeepsOldBehaviour(t *testing.T) {
	got := (&config.Limits{InFlightMode: modep(config.InFlightModeFixed)}).InFlight()
	if got.Min != 1000 || got.Max != 1000 || got.Start != 1000 {
		t.Fatalf("fixed default must be 1000: %+v", got)
	}
	got = (&config.Limits{InFlightMode: modep(config.InFlightModeFixed), MaxInFlight: intp(5000)}).InFlight()
	if got.Min != 5000 || got.Max != 5000 {
		t.Fatalf("fixed must use maxInFlight: %+v", got)
	}
}

func TestJSPoolBounds(t *testing.T) {
	var none *config.Limits
	if b := none.JSPool(); b.Min != 1 || !b.Auto() {
		t.Fatalf("default pool bounds %+v", b)
	}
	b := (&config.Limits{JSMinInstances: intp(2), JSMaxInstances: intp(8)}).JSPool()
	if b.Min != 2 || b.Max != 8 || b.Auto() {
		t.Fatalf("pool bounds %+v", b)
	}
}

func TestLimitRules(t *testing.T) {
	cases := []struct {
		name  string
		l     *config.Limits
		path  config.Pointer
		sev   config.Severity
		count int
	}{
		{"valid", &config.Limits{MinInFlight: intp(10), MaxInFlight: intp(20), JSMinInstances: intp(1), JSMaxInstances: intp(4)}, "", "", 0},
		{"floor above cap", &config.Limits{MinInFlight: intp(30), MaxInFlight: intp(20)}, config.PointerMinInFlight, config.SeverityError, 1},
		{"floor above auto cap", &config.Limits{MinInFlight: intp(20000)}, config.PointerMinInFlight, config.SeverityError, 1},
		{"floor in fixed mode", &config.Limits{MinInFlight: intp(10), InFlightMode: modep(config.InFlightModeFixed)}, config.PointerMinInFlight, config.SeverityWarning, 1},
		{"pool inverted", &config.Limits{JSMinInstances: intp(8), JSMaxInstances: intp(2)}, config.PointerJSMinInstances, config.SeverityError, 1},
		{"pool min with auto max", &config.Limits{JSMinInstances: intp(8)}, "", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			is := config.LimitRules{}.Check(c.l)
			if len(is) != c.count {
				t.Fatalf("issues %+v", is)
			}
			if c.count == 1 && (is[0].Path != c.path || is[0].Severity != c.sev || is[0].Code != config.IssueLimitOutOfRange) {
				t.Fatalf("issue %+v", is[0])
			}
		})
	}
}
