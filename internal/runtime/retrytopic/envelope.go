package retrytopic

import (
	"errors"
	"strconv"
	"time"

	"github.com/eventrouter/router/internal/dlq/record"
	"github.com/eventrouter/router/pkg/engine"
)

// HeaderKey names the router.retry.* headers of a tier record (the source position reuses record headers).
type HeaderKey string

const (
	HeaderAttempt         HeaderKey = "router.retry.attempt"
	HeaderDueAt           HeaderKey = "router.retry.due_at"
	HeaderErrorType       HeaderKey = "router.retry.error_type"
	HeaderErrorMessage    HeaderKey = "router.retry.error_message"
	HeaderSourceTimestamp HeaderKey = "router.retry.source_ts"
)

// NumberBase is the base of numeric header values.
type NumberBase int

const decimalBase NumberBase = 10

// ErrNotTierRecord is returned for records without a readable attempt / due time.
var ErrNotTierRecord = errors.New("retry tier record: router.retry headers missing or malformed")

// Envelope is one scheduled retry: the original source record plus attempt metadata.
type Envelope struct {
	Attempt   int
	DueAt     time.Time
	ErrorType record.ErrorType
	Message   string
	Source    engine.RawRecord
}

// EnvelopeCodec converts envelopes to tier records and back (millisecond timestamps).
type EnvelopeCodec struct{}

// Encode builds the tier record of e for topic; the record timestamp is now so tier retention starts at scheduling.
func (EnvelopeCodec) Encode(e Envelope, topic string, now time.Time) engine.RawRecord {
	src := e.Source
	headers := record.Headers{}.Strip(src.Headers)
	pairs := map[string]string{
		string(HeaderAttempt): strconv.Itoa(e.Attempt), string(HeaderDueAt): strconv.FormatInt(e.DueAt.UnixMilli(), int(decimalBase)),
		string(HeaderErrorType): string(e.ErrorType), string(HeaderErrorMessage): record.Truncator{}.Cut(e.Message, record.MaxMessageBytes),
		string(HeaderSourceTimestamp):    strconv.FormatInt(src.Timestamp.UnixMilli(), int(decimalBase)),
		string(record.HeaderSourceTopic): src.Topic, string(record.HeaderSourcePartition): strconv.Itoa(int(src.Partition)),
		string(record.HeaderSourceOffset): strconv.FormatInt(src.Offset, int(decimalBase)),
	}
	for _, k := range envelopeHeaderOrder {
		headers = append(headers, engine.Header{Key: k, Value: []byte(pairs[k])})
	}
	return engine.RawRecord{Key: src.Key, Value: src.Value, Headers: headers, Topic: topic, Timestamp: now}
}

// envelopeHeaderOrder keeps the written header order stable.
var envelopeHeaderOrder = []string{string(HeaderAttempt), string(HeaderDueAt), string(HeaderErrorType), string(HeaderErrorMessage),
	string(HeaderSourceTimestamp), string(record.HeaderSourceTopic), string(record.HeaderSourcePartition), string(record.HeaderSourceOffset)}

// Decode reads a tier record back; the source record gets its original position, timestamp and headers.
func (EnvelopeCodec) Decode(raw engine.RawRecord) (Envelope, error) {
	h := headerValues{}
	for _, x := range raw.Headers {
		h[x.Key] = string(x.Value)
	}
	attempt, err1 := strconv.Atoi(h[string(HeaderAttempt)])
	due, err2 := strconv.ParseInt(h[string(HeaderDueAt)], int(decimalBase), 64)
	if err1 != nil || err2 != nil || attempt < 1 {
		return Envelope{}, ErrNotTierRecord
	}
	src := engine.RawRecord{Key: raw.Key, Value: raw.Value, Headers: record.Headers{}.Strip(raw.Headers),
		Topic: h[string(record.HeaderSourceTopic)], Partition: int32(h.number(string(record.HeaderSourcePartition))),
		Offset: h.number(string(record.HeaderSourceOffset)), Timestamp: time.UnixMilli(h.number(string(HeaderSourceTimestamp)))}
	return Envelope{Attempt: attempt, DueAt: time.UnixMilli(due), ErrorType: record.ErrorType(h[string(HeaderErrorType)]),
		Message: h[string(HeaderErrorMessage)], Source: src}, nil
}

// headerValues indexes header values by key (last value wins).
type headerValues map[string]string

// number parses a decimal header (0 when absent or malformed).
func (h headerValues) number(k string) int64 {
	n, _ := strconv.ParseInt(h[k], int(decimalBase), 64)
	return n
}
