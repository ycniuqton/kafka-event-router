package boundary

import "time"

// Header is one record header (bytes as on the wire).
type Header struct {
	Key   string
	Value []byte
}

// GuestEvent is the event handed to user code.
type GuestEvent struct {
	Key       any
	Value     any
	Headers   []Header
	Topic     string
	Partition int32
	Offset    int64
	Timestamp time.Time
}

// Output is one event returned by user code; only key, value and headers are honoured.
type Output struct {
	Key        any
	KeySet     bool
	Value      any
	Headers    []Header
	HeadersSet bool
}

// Decoded is the validated result of a transform call.
type Decoded struct {
	Outputs []Output
	Dropped bool
}

// MarshallerKind names a boundary representation.
type MarshallerKind string

const (
	MarshallerJSON MarshallerKind = "json"
)

// Marshaller is the base of boundary representations (JSON in MVP, tagged in P2).
type Marshaller interface {
	Kind() MarshallerKind
	ToGuest(ev GuestEvent) ([]byte, error)
	FromGuest(raw []byte) (Decoded, error)
}

// OutputError reports a result that violates the transform contract.
type OutputError struct {
	Reason string
}

func (e *OutputError) Error() string { return e.Reason }

// Field names members of the guest event and output objects.
type Field string

const (
	FieldKey       Field = "key"
	FieldValue     Field = "value"
	FieldHeaders   Field = "headers"
	FieldTopic     Field = "topic"
	FieldPartition Field = "partition"
	FieldOffset    Field = "offset"
	FieldTimestamp Field = "timestamp"
	FieldBytes     Field = "$bytes"
)

// Reason is an OutputError message template.
type Reason string

const (
	ReasonNotObject     Reason = "transform must return an object, null or an array of objects (got %s)"
	ReasonItemNotObject Reason = "item %d of the returned array is not an object"
	ReasonBadHeaders    Reason = "headers must be an object of strings"
	ReasonBadHeader     Reason = "header %q must be a string or {\"$bytes\": base64}"
	ReasonUndefined     Reason = "transform returned undefined; return the event, null or an array"
)
