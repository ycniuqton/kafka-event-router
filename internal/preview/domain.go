// Package preview is the live-preview domain: sample sources and sessions, schema inference,
// traced preview runs on the shared engine and the DTOs the preview service returns.
package preview

import "time"

// SampleMode selects the sample source of a load.
type SampleMode string

const (
	ModeLatest    SampleMode = "latest"
	ModeTimeRange SampleMode = "time_range"
	ModeUpload    SampleMode = "upload"
	ModeDemo      SampleMode = "demo"
)

// EventStatus is the final outcome of one sample in a run (values match the engine trace).
type EventStatus string

const (
	StatusPassed   EventStatus = "passed"
	StatusFiltered EventStatus = "filtered"
	StatusError    EventStatus = "error"
	StatusDropped  EventStatus = "dropped"
)

// RunMode distinguishes an interactive preview from the Test button run.
type RunMode string

const (
	RunPreview RunMode = "preview"
	RunTest    RunMode = "test"
)

// SampleLimit holds the hard limits and defaults of sample loads and pages.
type SampleLimit int64

const (
	MaxEvents       SampleLimit = 1000
	MaxBytes        SampleLimit = 10 << 20
	DefaultCount    SampleLimit = 50
	DefaultPageSize SampleLimit = 100
	MaxPageSize     SampleLimit = 1000
	MaxInlineEvents SampleLimit = 1000
)

// SessionPolicy holds session lifetime and per-operation deadlines.
type SessionPolicy time.Duration

const (
	SessionTTL   SessionPolicy = SessionPolicy(time.Hour)
	LoadDeadline SessionPolicy = SessionPolicy(10 * time.Second)
	RunDeadline  SessionPolicy = SessionPolicy(10 * time.Second)
)

// Duration converts a policy value to a time.Duration.
func (p SessionPolicy) Duration() time.Duration { return time.Duration(p) }

// MaskStyle names a value masking style (maskers land in P2; the directive already carries it).
type MaskStyle string

const (
	MaskRedact         MaskStyle = "redact"
	MaskPartial        MaskStyle = "partial"
	MaskTypePreserving MaskStyle = "type_preserving"
)

// RawEncoding tells how raw bytes are rendered in a SampleDTO.
type RawEncoding string

const (
	EncodingUTF8   RawEncoding = "utf8"
	EncodingBase64 RawEncoding = "base64"
)

// JSONType is the inferred JSON type of a field.
type JSONType string

const (
	TypeString  JSONType = "string"
	TypeNumber  JSONType = "number"
	TypeBoolean JSONType = "boolean"
	TypeObject  JSONType = "object"
	TypeArray   JSONType = "array"
	TypeNull    JSONType = "null"
)

// SchemaLimit bounds schema inference.
type SchemaLimit int

const (
	SchemaTopValues      SchemaLimit = 20
	SchemaLowCardinality SchemaLimit = 50
	SchemaMaxDepth       SchemaLimit = 16
)

// SyntheticCoordinate fills Kafka coordinates of uploaded and inline events.
type SyntheticCoordinate int32

const SyntheticPartition SyntheticCoordinate = -1

// SyntheticTopic names the topic given to uploads without a topic.
type SyntheticTopic string

const UploadTopic SyntheticTopic = "upload"

// SessionIDPrefix starts every sample session id.
type SessionIDPrefix string

const SessionPrefix SessionIDPrefix = "ps_"
