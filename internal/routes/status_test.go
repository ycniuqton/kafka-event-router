package routes

import "testing"

func TestRouteStatusResolver(t *testing.T) {
	cases := []struct {
		name     string
		desired  DesiredState
		active   bool
		observed ObservedState
		want     Status
	}{
		{"archived wins", DesiredArchived, true, ObservedError, StatusArchived},
		{"never deployed", DesiredPaused, false, ObservedUnknown, StatusDraft},
		{"never deployed but running desired", DesiredRunning, false, ObservedRunning, StatusDraft},
		{"runtime error", DesiredRunning, true, ObservedError, StatusError},
		{"paused", DesiredPaused, true, ObservedPaused, StatusPaused},
		{"paused but runner still up", DesiredPaused, true, ObservedRunning, StatusPaused},
		{"running", DesiredRunning, true, ObservedRunning, StatusRunning},
		{"running unknown observed", DesiredRunning, true, ObservedUnknown, StatusRunning},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := (RouteStatusResolver{}).Resolve(c.desired, c.active, c.observed); got != c.want {
				t.Fatalf("got %s want %s", got, c.want)
			}
		})
	}
}
