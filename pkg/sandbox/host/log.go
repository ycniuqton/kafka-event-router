package host

import "sync"

// LogFn is ctx.log: appends one line to the per-call LogBuffer.
type LogFn struct{}

func (LogFn) Name() FnName { return FnLog }

func (LogFn) Doc() HelperDoc {
	return HelperDoc{Name: FnLog, Signature: "(...values: unknown[]) => void",
		Doc: "Writes a line to the preview Console tab; output is size-capped and never stored in production."}
}

// Invoke appends the already formatted line (first argument).
func (LogFn) Invoke(call *Call) (any, error) {
	line, err := ArgsOf(FnLog, call).String(0)
	if err != nil {
		return nil, err
	}
	if call.Logs != nil {
		call.Logs.Append(line)
	}
	return nil, nil
}

// LogBuffer collects log lines of one call up to a byte cap.
type LogBuffer struct {
	mu        sync.Mutex
	max       int64
	used      int64
	lines     []string
	truncated bool
}

// NewLogBuffer returns a buffer holding at most maxBytes of log text.
func NewLogBuffer(maxBytes int64) *LogBuffer {
	return &LogBuffer{max: maxBytes}
}

// Append adds line or, when it does not fit, discards it and flags truncation.
func (b *LogBuffer) Append(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used+int64(len(line)) > b.max {
		b.truncated = true
		return
	}
	b.used += int64(len(line))
	b.lines = append(b.lines, line)
}

// Lines returns the kept lines.
func (b *LogBuffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.lines...)
}

// Truncated reports whether any line was dropped.
func (b *LogBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
