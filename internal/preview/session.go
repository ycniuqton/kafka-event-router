package preview

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// SessionID identifies a sample session.
type SessionID string

// SessionIDBytes is the random part length of a session id.
type SessionIDBytes int

const SessionIDEntropy SessionIDBytes = 16

// NewSessionID returns a fresh random id with the session prefix.
func NewSessionID() SessionID {
	b := make([]byte, SessionIDEntropy)
	_, _ = rand.Read(b)
	return SessionID(string(SessionPrefix) + hex.EncodeToString(b))
}

// Session is the metadata of one loaded sample set; records are stored next to it.
type Session struct {
	ID           SessionID         `json:"id"`
	OwnerID      string            `json:"ownerId"`
	TeamID       string            `json:"teamId,omitempty"`
	ConnectionID string            `json:"connectionId,omitempty"`
	Topics       []string          `json:"topics"`
	Mode         SampleMode        `json:"mode"`
	Format       config.FormatType `json:"format"`
	Loaded       int               `json:"loaded"`
	Bytes        int64             `json:"bytes"`
	Truncated    bool              `json:"truncated"`
	StopReason   kafka.StopReason  `json:"stopReason"`
	Masked       bool              `json:"masked,omitempty"`
	MaskStyle    MaskStyle         `json:"maskStyle,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
	ExpiresAt    time.Time         `json:"expiresAt"`
}

// SampleRecord is one stored sample; raw bytes are kept so format overrides can re-decode.
type SampleRecord struct {
	Seq       int             `json:"seq"`
	Topic     string          `json:"topic"`
	Partition int32           `json:"partition"`
	Offset    int64           `json:"offset"`
	Timestamp time.Time       `json:"timestamp"`
	Key       []byte          `json:"key"`
	Value     []byte          `json:"value"`
	Headers   []engine.Header `json:"headers"`
}

// Size is the byte size counted against the session cap.
func (r SampleRecord) Size() int64 {
	size := int64(len(r.Key) + len(r.Value))
	for _, h := range r.Headers {
		size += int64(len(h.Key) + len(h.Value))
	}
	return size
}

// Raw returns the record as engine input.
func (r SampleRecord) Raw() engine.RawRecord {
	return engine.RawRecord{Key: r.Key, Value: r.Value, Headers: r.Headers, Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp}
}

// InlineEvent is an event given in a request (upload, inline run) in the event JSON shape.
type InlineEvent struct {
	Key       json.RawMessage    `json:"key,omitempty"`
	Value     json.RawMessage    `json:"value"`
	Headers   []engine.HeaderDoc `json:"headers,omitempty"`
	Timestamp *time.Time         `json:"timestamp,omitempty"`
	Topic     string             `json:"topic,omitempty"`
	Partition *int32             `json:"partition,omitempty"`
	Offset    *int64             `json:"offset,omitempty"`
}

// Caller is who asks, as stated by the API's signed directive.
type Caller struct {
	UserID    string    `json:"userId"`
	TeamID    string    `json:"teamId,omitempty"`
	Mask      bool      `json:"mask"`
	MaskStyle MaskStyle `json:"maskStyle,omitempty"`
}

// LoadRequest is the body of POST /preview/samples.
type LoadRequest struct {
	ConnectionID string            `json:"connectionId,omitempty"`
	Topics       []string          `json:"topics,omitempty"`
	Mode         SampleMode        `json:"mode"`
	Count        int               `json:"count,omitempty"`
	From         *time.Time        `json:"from,omitempty"`
	To           *time.Time        `json:"to,omitempty"`
	Format       config.FormatType `json:"format,omitempty"`
	Events       []InlineEvent     `json:"events,omitempty"`
	Upload       string            `json:"upload,omitempty"`
}

// EffectiveCount returns the requested count clamped to the hard limit (default when unset).
func (r LoadRequest) EffectiveCount() int {
	if r.Count <= 0 {
		return int(DefaultCount)
	}
	return min(r.Count, int(MaxEvents))
}

// EffectiveFormat returns the value format, JSON by default.
func (r LoadRequest) EffectiveFormat() config.FormatType {
	if r.Format == "" {
		return config.FormatTypeJSON
	}
	return r.Format
}

// LoadResult is what a sample source produced.
type LoadResult struct {
	Records    []SampleRecord
	Bytes      int64
	StopReason kafka.StopReason
}

// Page addresses one page of session events with an optional text query.
type Page struct {
	Cursor Cursor
	Limit  int
	Query  string
}

// EffectiveLimit returns the page size clamped to its bounds.
func (p Page) EffectiveLimit() int {
	if p.Limit <= 0 {
		return int(DefaultPageSize)
	}
	return min(p.Limit, int(MaxPageSize))
}

// Cursor is the opaque position of the next page.
type Cursor string
