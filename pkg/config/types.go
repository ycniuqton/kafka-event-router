package config

// Severity distinguishes blocking errors from advisory warnings.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// LimitDefault holds the engine defaults applied when a route omits a limit.
type LimitDefault int

const (
	LimitDefaultJSTimeoutMs        LimitDefault = 50
	LimitDefaultJSMemoryMb         LimitDefault = 64
	LimitDefaultMaxInFlight        LimitDefault = 1000
	LimitDefaultMaxOutputsPerEvent LimitDefault = 100
	LimitDefaultMaxOutputBytes     LimitDefault = 1048576
)

// IsEnabled reports whether the step runs; absent means enabled.
func (s Step) IsEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}

// IsStrict reports whether missing fields are step errors; absent means lenient.
func (s Step) IsStrict() bool {
	return s.Strict != nil && *s.Strict
}

// Resolved returns the limits with defaults applied to every omitted value.
func (l *Limits) Resolved() ResolvedLimits {
	r := ResolvedLimits{
		JSTimeoutMs:        int(LimitDefaultJSTimeoutMs),
		JSMemoryMb:         int(LimitDefaultJSMemoryMb),
		MaxInFlight:        int(LimitDefaultMaxInFlight),
		MaxOutputsPerEvent: int(LimitDefaultMaxOutputsPerEvent),
		MaxOutputBytes:     int(LimitDefaultMaxOutputBytes),
	}
	if l == nil {
		return r
	}
	r.JSTimeoutMs = r.or(l.JSTimeoutMs, r.JSTimeoutMs)
	r.JSMemoryMb = r.or(l.JSMemoryMb, r.JSMemoryMb)
	r.MaxInFlight = r.or(l.MaxInFlight, r.MaxInFlight)
	r.MaxOutputsPerEvent = r.or(l.MaxOutputsPerEvent, r.MaxOutputsPerEvent)
	r.MaxOutputBytes = r.or(l.MaxOutputBytes, r.MaxOutputBytes)
	return r
}

// ResolvedLimits are route limits with every default applied.
type ResolvedLimits struct {
	JSTimeoutMs        int
	JSMemoryMb         int
	MaxInFlight        int
	MaxOutputsPerEvent int
	MaxOutputBytes     int
}

// or returns the configured value or the default when it is absent.
func (ResolvedLimits) or(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

// DLQConnection returns the DLQ connection; absent means the source connection.
func (c *RouteConfig) DLQConnection() string {
	if c.ErrorPolicy != nil && c.ErrorPolicy.DLQConnectionID != nil {
		return *c.ErrorPolicy.DLQConnectionID
	}
	return c.Source.ConnectionID
}
