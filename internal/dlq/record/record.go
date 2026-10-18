// Package record is the shared dead-letter record contract: the router.* headers the worker
// writes next to the original key/value/headers, and the codec the API reads them back with.
package record

import (
	"strings"
	"time"

	"github.com/eventrouter/router/pkg/engine"
)

// HeaderKey names one router.* header of a DLQ record.
type HeaderKey string

const (
	HeaderRouteID         HeaderKey = "router.route_id"
	HeaderVersion         HeaderKey = "router.version"
	HeaderBranch          HeaderKey = "router.branch"
	HeaderStep            HeaderKey = "router.step"
	HeaderErrorType       HeaderKey = "router.error_type"
	HeaderErrorMessage    HeaderKey = "router.error_message"
	HeaderStack           HeaderKey = "router.stack"
	HeaderSourceTopic     HeaderKey = "router.source_topic"
	HeaderSourcePartition HeaderKey = "router.source_partition"
	HeaderSourceOffset    HeaderKey = "router.source_offset"
	HeaderFailedAt        HeaderKey = "router.failed_at"
	HeaderDelivered       HeaderKey = "router.delivered_branches"
)

// ListSeparator joins multi-value header values (router.delivered_branches).
type ListSeparator string

const BranchListSeparator ListSeparator = ","

// HeaderNamespace is the prefix reserved for router metadata headers (stripped from originals).
type HeaderNamespace string

const RouterNamespace HeaderNamespace = "router."

// ErrorType classifies why an event was dead-lettered.
type ErrorType string

const (
	ErrorDecode        ErrorType = "decode_error"
	ErrorStep          ErrorType = "step_error"
	ErrorTimeout       ErrorType = "timeout"
	ErrorOutputInvalid ErrorType = "output_invalid"
	ErrorUnknown       ErrorType = "unknown"
)

// ErrorTypes lists every error type a writer may emit (ErrorUnknown is reader-only).
var ErrorTypes = []ErrorType{ErrorDecode, ErrorStep, ErrorTimeout, ErrorOutputInvalid}

// Valid reports whether t is a writer error type.
func (t ErrorType) Valid() bool {
	for _, k := range ErrorTypes {
		if k == t {
			return true
		}
	}
	return false
}

// EngineErrorTypes maps engine processing error types to DLQ error types.
var EngineErrorTypes = map[engine.ErrorType]ErrorType{
	engine.ErrDecode:          ErrorDecode,
	engine.ErrDecodeTransient: ErrorDecode,
	engine.ErrStep:            ErrorStep,
	engine.ErrStepTimeout:     ErrorTimeout,
	engine.ErrOutputInvalid:   ErrorOutputInvalid,
	engine.ErrLimit:           ErrorStep,
}

// FromEngine returns the DLQ error type of an engine error type (step_error when unmapped).
func FromEngine(t engine.ErrorType) ErrorType {
	if k, ok := EngineErrorTypes[t]; ok {
		return k
	}
	return ErrorStep
}

// Limit bounds free-text header sizes; Encode truncates longer values.
type Limit int

const (
	MaxMessageBytes Limit = 2048
	MaxStackBytes   Limit = 8192
)

// UnreadableMessage is the message shown for a DLQ record whose router headers cannot be read.
type UnreadableMessage string

const MessageUnreadable UnreadableMessage = "unreadable DLQ record"

// Ref is the identity of a record inside its DLQ topic.
type Ref struct {
	Topic     string
	Partition int32
	Offset    int64
}

// SourcePosition is where the failed event was read from.
type SourcePosition struct {
	Topic     string
	Partition int32
	Offset    int64
}

// Failure describes why and where processing failed.
type Failure struct {
	RouteID   string
	VersionNo int
	Branch    string
	Step      string
	ErrorType ErrorType
	Message   string
	Stack     string
	FailedAt  time.Time
	Delivered BranchSet
}

// BranchSet is a comma-separated list of branch ids (kept a string so Failure stays comparable).
type BranchSet string

// BranchSetOf joins branch ids.
func BranchSetOf(branches ...string) BranchSet {
	return BranchSet(strings.Join(branches, string(BranchListSeparator)))
}

// Has reports whether branch is in the set.
func (s BranchSet) Has(branch string) bool {
	for _, b := range strings.Split(string(s), string(BranchListSeparator)) {
		if strings.TrimSpace(b) == branch && branch != "" {
			return true
		}
	}
	return false
}

// WasDelivered reports whether the worker already delivered branch before the failure (branch-aware retry).
func (f Failure) WasDelivered(branch string) bool { return f.Delivered.Has(branch) }

// Record is one dead letter: original bytes and headers plus failure metadata.
type Record struct {
	Ref     Ref
	Key     []byte
	Value   []byte
	Headers []engine.Header
	Failure Failure
	Source  SourcePosition
}

// Size is the byte size of key, value and original headers (scan budgets).
func (r Record) Size() int64 {
	size := int64(len(r.Key) + len(r.Value))
	for _, h := range r.Headers {
		size += int64(len(h.Key) + len(h.Value))
	}
	return size
}
