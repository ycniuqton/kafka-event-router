package sandbox

// EngineKind names a sandbox engine implementation selected by configuration.
type EngineKind string

const (
	EngineQuickJSWazero EngineKind = "quickjs_wazero"
)

// EntryKind is the contract user code must satisfy.
type EntryKind string

const (
	EntryTransform EntryKind = "transform"
	EntryPredicate EntryKind = "predicate"
)

// ErrorKind classifies a failed check or call; it drives retry and pool decisions.
type ErrorKind string

const (
	ErrorCompile        ErrorKind = "compile"
	ErrorRuntime        ErrorKind = "runtime"
	ErrorTimeout        ErrorKind = "timeout"
	ErrorMemory         ErrorKind = "memory"
	ErrorStackOverflow  ErrorKind = "stack_overflow"
	ErrorOutputTooLarge ErrorKind = "output_too_large"
	ErrorInvalidOutput  ErrorKind = "invalid_output"
	ErrorHost           ErrorKind = "host"
	ErrorInternal       ErrorKind = "internal"
	ErrorDeterminism    ErrorKind = "determinism_mismatch"
)

// ResetPolicy decides how guest state is reset between calls.
type ResetPolicy string

const (
	ResetFreshContext  ResetPolicy = "fresh_context"
	ResetLockdownReuse ResetPolicy = "lockdown_reuse"
)

// ScopeMode tells the sandbox who is calling; it decides log retention.
type ScopeMode string

const (
	ScopeWorker  ScopeMode = "worker"
	ScopePreview ScopeMode = "preview"
	ScopeTest    ScopeMode = "test"
)

// KeepsLogs reports whether ctx.log output is returned to the caller in this mode.
func (m ScopeMode) KeepsLogs() bool {
	return m != ScopeWorker
}

// Severity grades a Diagnostic.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// PoolAction is what happens to an instance after a call.
type PoolAction string

const (
	PoolRelease PoolAction = "release"
	PoolDiscard PoolAction = "discard"
)
