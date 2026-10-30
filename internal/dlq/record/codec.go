package record

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eventrouter/router/pkg/engine"
)

// ErrUnreadable is returned with a best-effort record whose router headers are missing or malformed.
var ErrUnreadable = errors.New("dlq record: router headers missing or malformed")

// NumberBase is the decimal base used for numeric header values.
type NumberBase int

const decimal NumberBase = 10

// TimeLayout is the header format of router.failed_at.
type TimeLayout string

const FailedAtLayout TimeLayout = time.RFC3339Nano

// Codec converts between Record and Kafka bytes; it is kgo-agnostic (engine.RawRecord).
type Codec struct{}

// Encode builds the DLQ record: original key/value, original headers (router.* removed) plus metadata.
func (Codec) Encode(r Record) engine.RawRecord {
	headers := Headers{}.Strip(r.Headers)
	return engine.RawRecord{Key: r.Key, Value: r.Value, Headers: append(headers, Headers{}.Metadata(r)...)}
}

// Decode reads a DLQ record; unreadable metadata yields ErrUnreadable with error type unknown.
func (Codec) Decode(raw engine.RawRecord) (Record, error) {
	h := NewHeaderReader(raw.Headers)
	rec := Record{
		Ref:     Ref{Topic: raw.Topic, Partition: raw.Partition, Offset: raw.Offset},
		Key:     raw.Key,
		Value:   raw.Value,
		Headers: Headers{}.Strip(raw.Headers),
		Failure: h.Failure(),
		Source:  h.Source(),
	}
	if rec.Failure.FailedAt.IsZero() {
		rec.Failure.FailedAt = raw.Timestamp
	}
	if h.Broken() {
		rec.Failure.ErrorType = ErrorUnknown
		rec.Failure.Message = string(MessageUnreadable)
		return rec, ErrUnreadable
	}
	return rec, nil
}

// Headers builds and filters header lists.
type Headers struct{}

// Strip returns the headers outside the router namespace.
func (Headers) Strip(in []engine.Header) []engine.Header {
	out := make([]engine.Header, 0, len(in))
	for _, h := range in {
		if !strings.HasPrefix(h.Key, string(RouterNamespace)) {
			out = append(out, h)
		}
	}
	return out
}

// Metadata returns the router.* headers of r; message and stack are truncated to their limits.
func (Headers) Metadata(r Record) []engine.Header {
	f, s := r.Failure, r.Source
	pairs := []struct {
		key   HeaderKey
		value string
	}{
		{HeaderRouteID, f.RouteID},
		{HeaderVersion, strconv.Itoa(f.VersionNo)},
		{HeaderBranch, f.Branch},
		{HeaderStep, f.Step},
		{HeaderErrorType, string(f.ErrorType)},
		{HeaderErrorMessage, Truncator{}.Cut(f.Message, MaxMessageBytes)},
		{HeaderStack, Truncator{}.Cut(f.Stack, MaxStackBytes)},
		{HeaderSourceTopic, s.Topic},
		{HeaderSourcePartition, strconv.FormatInt(int64(s.Partition), int(decimal))},
		{HeaderSourceOffset, strconv.FormatInt(s.Offset, int(decimal))},
		{HeaderFailedAt, f.FailedAt.UTC().Format(string(FailedAtLayout))},
	}
	out := make([]engine.Header, 0, len(pairs)+2)
	for _, p := range pairs {
		out = append(out, engine.Header{Key: string(p.key), Value: []byte(p.value)})
	}
	if f.Delivered != "" {
		out = append(out, engine.Header{Key: string(HeaderDelivered), Value: []byte(f.Delivered)})
	}
	if f.Attempts > 0 {
		out = append(out, engine.Header{Key: string(HeaderRetryAttempts), Value: []byte(strconv.Itoa(f.Attempts))})
	}
	return out
}

// Truncator cuts strings to a byte limit without splitting a UTF-8 rune.
type Truncator struct{}

// Cut returns s limited to max bytes.
func (Truncator) Cut(s string, max Limit) string {
	if len(s) <= int(max) {
		return s
	}
	cut := int(max)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// HeaderReader parses router.* headers and remembers whether any required one was unreadable.
type HeaderReader struct {
	values map[HeaderKey]string
	broken bool
}

// NewHeaderReader indexes the router headers of a record (last value wins).
func NewHeaderReader(in []engine.Header) *HeaderReader {
	r := &HeaderReader{values: map[HeaderKey]string{}}
	for _, h := range in {
		if strings.HasPrefix(h.Key, string(RouterNamespace)) {
			r.values[HeaderKey(h.Key)] = string(h.Value)
		}
	}
	return r
}

// Broken reports whether a required header was missing or malformed.
func (r *HeaderReader) Broken() bool { return r.broken }

// Failure reads the failure metadata.
func (r *HeaderReader) Failure() Failure {
	et := ErrorType(r.required(HeaderErrorType))
	if !et.Valid() {
		r.broken = true
	}
	return Failure{
		RouteID:   r.required(HeaderRouteID),
		VersionNo: int(r.number(HeaderVersion)),
		Branch:    r.values[HeaderBranch],
		Step:      r.values[HeaderStep],
		ErrorType: et,
		Message:   r.values[HeaderErrorMessage],
		Stack:     r.values[HeaderStack],
		FailedAt:  r.time(HeaderFailedAt),
		Delivered: BranchSet(r.values[HeaderDelivered]),
		Attempts:  int(r.number(HeaderRetryAttempts)),
	}
}

// Source reads the source position.
func (r *HeaderReader) Source() SourcePosition {
	return SourcePosition{
		Topic:     r.values[HeaderSourceTopic],
		Partition: int32(r.number(HeaderSourcePartition)),
		Offset:    r.number(HeaderSourceOffset),
	}
}

func (r *HeaderReader) required(k HeaderKey) string {
	v, ok := r.values[k]
	if !ok || v == "" {
		r.broken = true
	}
	return v
}

func (r *HeaderReader) number(k HeaderKey) int64 {
	v, ok := r.values[k]
	if !ok {
		return 0
	}
	n, err := strconv.ParseInt(v, int(decimal), 64)
	if err != nil {
		r.broken = true
	}
	return n
}

func (r *HeaderReader) time(k HeaderKey) time.Time {
	v, ok := r.values[k]
	if !ok {
		return time.Time{}
	}
	t, err := time.Parse(string(FailedAtLayout), v)
	if err != nil {
		r.broken = true
	}
	return t
}

// TopicPrefix is the prefix of default DLQ topic names.
type TopicPrefix string

const DefaultTopicPrefix TopicPrefix = "router.dlq."

// TopicNaming derives the default DLQ topic of a route (errorPolicy.dlqTopic absent).
type TopicNaming struct{}

// Default returns router.dlq.<slug(routeName)>; characters Kafka rejects become '-'.
func (TopicNaming) Default(routeName string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(routeName) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-' {
			b.WriteRune(c)
			continue
		}
		b.WriteRune('-')
	}
	return string(DefaultTopicPrefix) + b.String()
}
