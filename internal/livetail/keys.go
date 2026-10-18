// Package livetail streams a sample of a running route's records to browser sockets: workers copy
// finished records into a capped Redis stream while somebody watches, the API relays them (P3).
package livetail

import (
	"strings"
	"time"
)

// KeyPrefix names the Redis key families of the live tail.
type KeyPrefix string

const (
	PrefixFlag   KeyPrefix = "router:tail:flag:"
	PrefixStream KeyPrefix = "router:tail:stream:"
)

// TailKey builds the Redis keys of one route's tail.
type TailKey struct {
	RouteID string
}

// Flag is the key whose presence means "somebody is watching this route".
func (k TailKey) Flag() string { return string(PrefixFlag) + k.RouteID }

// Stream is the capped stream the workers append to and the API reads.
func (k TailKey) Stream() string { return string(PrefixStream) + k.RouteID }

// RouteOf returns the route id of a flag or stream key ("" when the key is not a tail key).
func (TailKey) RouteOf(key string) string {
	for _, p := range []KeyPrefix{PrefixFlag, PrefixStream} {
		if id, ok := strings.CutPrefix(key, string(p)); ok {
			return id
		}
	}
	return ""
}

// TailLimits bounds the cost of the live tail (design §1.3).
type TailLimits struct {
	RatePerSec    int
	Burst         int
	FlagTTL       time.Duration
	RenewEvery    time.Duration
	FlagCacheTTL  time.Duration
	StreamMaxLen  int64
	StreamTTL     time.Duration
	ReadBlock     time.Duration
	ReadCount     int64
	MaxValueBytes int
	QueueSize     int
	ThrottleEvery time.Duration
	RedisTimeout  time.Duration
}

// DefaultTailLimits are the limits of the design: 20 records/s per route, flag TTL 60 s renewed every 20 s.
func DefaultTailLimits() TailLimits {
	return TailLimits{
		RatePerSec: 20, Burst: 20, FlagTTL: 60 * time.Second, RenewEvery: 20 * time.Second,
		FlagCacheTTL: 2 * time.Second, StreamMaxLen: 1000, StreamTTL: 120 * time.Second,
		ReadBlock: 5 * time.Second, ReadCount: 100, MaxValueBytes: 64 << 10, QueueSize: 256,
		ThrottleEvery: time.Second, RedisTimeout: 2 * time.Second,
	}
}

// TailOutcome is how a record finished, as the live tail shows it.
type TailOutcome string

const (
	OutcomeSent         TailOutcome = "sent"
	OutcomeFiltered     TailOutcome = "filtered"
	OutcomeDropped      TailOutcome = "dropped"
	OutcomeDeadLettered TailOutcome = "dead_lettered"
	OutcomeError        TailOutcome = "error"
)

// AllOutcomes lists every outcome (client filters are validated against it).
var AllOutcomes = []TailOutcome{OutcomeSent, OutcomeFiltered, OutcomeDropped, OutcomeDeadLettered, OutcomeError}

// Valid reports whether o is a known outcome.
func (o TailOutcome) Valid() bool {
	for _, k := range AllOutcomes {
		if k == o {
			return true
		}
	}
	return false
}

// Encoding tells how a tailed payload is carried: decoded JSON or plain text.
type Encoding string

const (
	EncodingJSON   Encoding = "json"
	EncodingText   Encoding = "text"
	EncodingBase64 Encoding = "base64"
)

// Header is one record header as text.
type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// TailEvent is one record (input or output) of a tail entry.
type TailEvent struct {
	Topic     string   `json:"topic,omitempty"`
	Partition int32    `json:"partition"`
	Offset    int64    `json:"offset"`
	Branch    string   `json:"branch,omitempty"`
	Key       any      `json:"key,omitempty"`
	Value     any      `json:"value"`
	Encoding  Encoding `json:"encoding"`
	Headers   []Header `json:"headers,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
}

// TailRecord is one finished source record with its outputs (design §1.3).
type TailRecord struct {
	At        time.Time   `json:"at"`
	Partition int32       `json:"partition"`
	Offset    int64       `json:"offset"`
	Branch    string      `json:"branch,omitempty"`
	Outcome   TailOutcome `json:"outcome"`
	Step      string      `json:"step,omitempty"`
	ErrorType string      `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
	Input     TailEvent   `json:"input"`
	Outputs   []TailEvent `json:"outputs"`
}

// EntryKind tells what a stream entry carries: a record, or a throttle notice from a worker.
type EntryKind string

const (
	EntryRecord    EntryKind = "record"
	EntryThrottled EntryKind = "throttled"
)

// EntryField names the fields of a stream entry.
type EntryField string

const (
	FieldKind EntryField = "kind"
	FieldData EntryField = "data"
)
