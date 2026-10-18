package routes

// RouteStatusResolver derives the badge status from desired state, deployment and runtime.
type RouteStatusResolver struct{}

// Resolve applies the precedence archived > draft > error > paused > running.
func (RouteStatusResolver) Resolve(desired DesiredState, hasActive bool, observed ObservedState) Status {
	switch {
	case desired == DesiredArchived:
		return StatusArchived
	case !hasActive:
		return StatusDraft
	case observed == ObservedError:
		return StatusError
	case desired == DesiredPaused:
		return StatusPaused
	default:
		return StatusRunning
	}
}
