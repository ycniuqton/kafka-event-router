package sandbox

import "time"

// ByteUnit scales byte quantities used by limits.
type ByteUnit int64

const (
	Byte ByteUnit = 1
	KiB  ByteUnit = 1024 * Byte
	MiB  ByteUnit = 1024 * KiB
)

// Limits bounds the resources one sandbox call may consume.
type Limits struct {
	Timeout        time.Duration
	MemoryBytes    int64
	MaxOutputBytes int64
	MaxLogBytes    int64
	MaxStackBytes  int64
	MaxSourceBytes int64
}

// LimitDefault is used when a route does not request a value.
var LimitDefault = Limits{
	Timeout:        50 * time.Millisecond,
	MemoryBytes:    int64(64 * MiB),
	MaxOutputBytes: int64(1 * MiB),
	MaxLogBytes:    int64(16 * KiB),
	MaxStackBytes:  int64(256 * KiB),
	MaxSourceBytes: int64(64 * KiB),
}

// LimitMaximum holds the operator caps (env-overridable through Config).
var LimitMaximum = Limits{
	Timeout:        1000 * time.Millisecond,
	MemoryBytes:    int64(256 * MiB),
	MaxOutputBytes: int64(8 * MiB),
	MaxLogBytes:    int64(64 * KiB),
	MaxStackBytes:  int64(256 * KiB),
	MaxSourceBytes: int64(64 * KiB),
}

// RouteLimits are the values a route document requests; zero means "use the default".
type RouteLimits struct {
	JSTimeoutMs   int64 `json:"jsTimeoutMs,omitempty"`
	JSMemoryMb    int64 `json:"jsMemoryMb,omitempty"`
	JSMaxOutputKb int64 `json:"jsMaxOutputKb,omitempty"`
}

// LimitPath is the JSON Pointer of a route limit field.
type LimitPath string

const (
	PathJSTimeoutMs   LimitPath = "/limits/jsTimeoutMs"
	PathJSMemoryMb    LimitPath = "/limits/jsMemoryMb"
	PathJSMaxOutputKb LimitPath = "/limits/jsMaxOutputKb"
)

// LimitPolicy turns requested route limits into effective limits clamped to operator maxima.
type LimitPolicy struct {
	Defaults Limits
	Maxima   Limits
}

// NewLimitPolicy returns the policy with package defaults and the given maxima.
func NewLimitPolicy(maxima Limits) LimitPolicy {
	return LimitPolicy{Defaults: LimitDefault, Maxima: maxima}
}

// Resolve returns effective limits plus a warning per value that had to be clamped.
func (p LimitPolicy) Resolve(req RouteLimits) (Limits, []Diagnostic) {
	out := p.Defaults
	var issues []Diagnostic
	timeout := limitField{path: PathJSTimeoutMs, requested: req.JSTimeoutMs, unit: int64(time.Millisecond)}
	memory := limitField{path: PathJSMemoryMb, requested: req.JSMemoryMb, unit: int64(MiB)}
	output := limitField{path: PathJSMaxOutputKb, requested: req.JSMaxOutputKb, unit: int64(KiB)}
	ms, d := timeout.resolve(int64(p.Defaults.Timeout), int64(p.Maxima.Timeout))
	out.Timeout, issues = time.Duration(ms), d.appendTo(issues)
	out.MemoryBytes, d = memory.resolve(p.Defaults.MemoryBytes, p.Maxima.MemoryBytes)
	issues = d.appendTo(issues)
	out.MaxOutputBytes, d = output.resolve(p.Defaults.MaxOutputBytes, p.Maxima.MaxOutputBytes)
	issues = d.appendTo(issues)
	out.MaxLogBytes = min(out.MaxLogBytes, p.Maxima.MaxLogBytes)
	return out, issues
}

// limitField is one requested route limit expressed in its document unit.
type limitField struct {
	path      LimitPath
	requested int64
	unit      int64
}

// resolve converts the request to base units, clamps it and reports a clamp as a warning.
func (f limitField) resolve(def, max int64) (int64, optionalDiagnostic) {
	if f.requested <= 0 {
		return def, optionalDiagnostic{}
	}
	want := f.requested * f.unit
	if want <= max {
		return want, optionalDiagnostic{}
	}
	return max, optionalDiagnostic{set: true, d: Diagnostic{
		Severity: SeverityWarning,
		Path:     string(f.path),
		Message:  MsgLimitClamped.Format(f.requested, max/f.unit),
	}}
}

// optionalDiagnostic carries zero or one diagnostic.
type optionalDiagnostic struct {
	set bool
	d   Diagnostic
}

func (o optionalDiagnostic) appendTo(list []Diagnostic) []Diagnostic {
	if !o.set {
		return list
	}
	return append(list, o.d)
}
