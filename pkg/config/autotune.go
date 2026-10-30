package config

// AutoTuneDefault holds the defaults of adaptive in-flight and JS pool sizing (route-runtime 00003-autotune,
// js-sandbox 00001-pool-autoscale) applied when a route omits the value.
type AutoTuneDefault int

const (
	AutoTuneMinInFlight    AutoTuneDefault = 64
	AutoTuneMaxInFlight    AutoTuneDefault = 10000
	AutoTuneStartInFlight  AutoTuneDefault = AutoTuneDefault(LimitDefaultMaxInFlight)
	AutoTuneJSMinInstances AutoTuneDefault = 1
	AutoTuneJSMaxAuto      AutoTuneDefault = 0
)

// Pointers of the limit members the autotune rules report on.
const (
	PointerLimits         Pointer = "/limits"
	PointerMinInFlight    Pointer = PointerLimits + "/minInFlight"
	PointerJSMinInstances Pointer = PointerLimits + "/jsMinInstances"
)

// InFlightBounds is the resolved in-flight sizing of one route: the controller mode and the floor, cap and
// starting value of its limit (Min = Max = Start in fixed mode).
type InFlightBounds struct {
	Mode  InFlightMode
	Min   int
	Max   int
	Start int
}

// InFlight resolves the in-flight bounds: auto by default, maxInFlight as the cap (auto) or the size (fixed).
func (l *Limits) InFlight() InFlightBounds {
	if l.inFlightMode() == InFlightModeFixed {
		size := l.Resolved().MaxInFlight
		return InFlightBounds{Mode: InFlightModeFixed, Min: size, Max: size, Start: size}
	}
	capValue := int(AutoTuneMaxInFlight)
	floor := int(AutoTuneMinInFlight)
	if l != nil && l.MaxInFlight != nil {
		capValue = *l.MaxInFlight
	}
	if l != nil && l.MinInFlight != nil {
		floor = *l.MinInFlight
	}
	floor = min(floor, capValue)
	return InFlightBounds{Mode: InFlightModeAuto, Min: floor, Max: capValue, Start: min(max(int(AutoTuneStartInFlight), floor), capValue)}
}

// inFlightMode returns the configured mode; absent means auto.
func (l *Limits) inFlightMode() InFlightMode {
	if l == nil || l.InFlightMode == nil {
		return InFlightModeAuto
	}
	return *l.InFlightMode
}

// JSPoolBounds is the requested JS pool size per JS step; Max 0 means auto (the worker's cores).
type JSPoolBounds struct {
	Min int
	Max int
}

// JSPool resolves the JS pool bounds requested by the route.
func (l *Limits) JSPool() JSPoolBounds {
	b := JSPoolBounds{Min: int(AutoTuneJSMinInstances), Max: int(AutoTuneJSMaxAuto)}
	if l == nil {
		return b
	}
	if l.JSMinInstances != nil {
		b.Min = *l.JSMinInstances
	}
	if l.JSMaxInstances != nil {
		b.Max = *l.JSMaxInstances
	}
	return b
}

// Auto reports whether the pool cap follows the worker's cores.
func (b JSPoolBounds) Auto() bool { return b.Max == int(AutoTuneJSMaxAuto) }

// LimitRules checks the cross-field autotune limits (the schema checks each value's range).
type LimitRules struct{}

// Check returns an error per inverted range and a warning for a floor that fixed mode ignores.
func (LimitRules) Check(l *Limits) []Issue {
	if l == nil {
		return nil
	}
	var issues []Issue
	if l.MinInFlight != nil {
		issues = append(issues, LimitRules{}.inFlightFloor(l)...)
	}
	pool := l.JSPool()
	if !pool.Auto() && pool.Min > pool.Max {
		issues = append(issues, NewError(IssueLimitOutOfRange, PointerJSMinInstances,
			"jsMinInstances %d is above jsMaxInstances %d", pool.Min, pool.Max))
	}
	return issues
}

// inFlightFloor checks minInFlight against the cap (auto) or flags it as unused (fixed).
func (LimitRules) inFlightFloor(l *Limits) []Issue {
	if l.inFlightMode() == InFlightModeFixed {
		return []Issue{NewWarning(IssueLimitOutOfRange, PointerMinInFlight, "minInFlight is ignored when inFlightMode is fixed")}
	}
	capValue := int(AutoTuneMaxInFlight)
	if l.MaxInFlight != nil {
		capValue = *l.MaxInFlight
	}
	if *l.MinInFlight > capValue {
		return []Issue{NewError(IssueLimitOutOfRange, PointerMinInFlight, "minInFlight %d is above the in-flight cap %d", *l.MinInFlight, capValue)}
	}
	return nil
}
