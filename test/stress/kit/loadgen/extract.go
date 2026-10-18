package loadgen

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/eventrouter/router/test/stress/kit/payload"
	"github.com/eventrouter/router/test/stress/kit/stream"
)

// SeqSource names where the measurer reads a record's stream and sequence number (loadgen --seq-source).
type SeqSource string

const (
	SeqFromHeader  SeqSource = "header"
	SeqFromPayload SeqSource = "payload"
)

// Stamp is what the measurer extracts from one output record.
type Stamp struct {
	SentNs int64
	Stream string
	Seq    int64
	HasSeq bool
}

// Extractor reads the stream/seq identity of a record; the send time always comes from the header.
type Extractor interface {
	Identity(r *kgo.Record, s *Stamp)
}

// headerExtractor reads stress-stream / stress-seq headers (survive any value transform that keeps headers).
type headerExtractor struct{}

// Identity reads the identity headers.
func (headerExtractor) Identity(r *kgo.Record, s *Stamp) {
	for _, h := range r.Headers {
		switch stream.Header(h.Key) {
		case stream.HeaderStream:
			s.Stream = string(h.Value)
		case stream.HeaderSeq:
			if v, err := strconv.ParseInt(string(h.Value), 10, 64); err == nil {
				s.Seq, s.HasSeq = v, true
			}
		}
	}
}

// payloadExtractor scans the JSON value for top-level "seq" / "stream" members without decoding the document.
type payloadExtractor struct {
	seqKey, streamKey []byte
}

// Identity scans the value.
func (p payloadExtractor) Identity(r *kgo.Record, s *Stamp) {
	if i := bytes.Index(r.Value, p.seqKey); i >= 0 {
		rest := r.Value[i+len(p.seqKey):]
		end := bytes.IndexAny(rest, ",}")
		if v, err := strconv.ParseInt(string(bytes.TrimSpace(rest[:max(end, 0)])), 10, 64); err == nil {
			s.Seq, s.HasSeq = v, true
		}
	}
	if i := bytes.Index(r.Value, p.streamKey); i >= 0 {
		rest := r.Value[i+len(p.streamKey):]
		if end := bytes.IndexByte(rest, '"'); end >= 0 {
			s.Stream = string(rest[:end])
		}
	}
}

// ExtractorManager selects the configured extractor.
type ExtractorManager struct{}

var extractors = map[SeqSource]Extractor{
	SeqFromHeader:  headerExtractor{},
	SeqFromPayload: payloadExtractor{seqKey: []byte(`"` + payload.FieldSeq + `":`), streamKey: []byte(`"` + payload.FieldStream + `":"`)},
}

// For returns the extractor of a source.
func (ExtractorManager) For(src SeqSource) (Extractor, error) {
	e, ok := extractors[src]
	if !ok {
		return nil, fmt.Errorf("unknown seq source %q (header|payload)", src)
	}
	return e, nil
}

// SentTime reads the stress-sent-ns header; false when absent or malformed.
func (ExtractorManager) SentTime(r *kgo.Record) (int64, bool) {
	for _, h := range r.Headers {
		if stream.Header(h.Key) == stream.HeaderSentNs {
			v, err := strconv.ParseInt(string(h.Value), 10, 64)
			return v, err == nil
		}
	}
	return 0, false
}

// TrackBy names the identity of a delivery stream (loadgen --track-by): per producer stream, or per output topic and
// stream (fan-out: every output topic must receive every event).
type TrackBy string

const (
	TrackStream      TrackBy = "stream"
	TrackTopicStream TrackBy = "topic-stream"
)

// trackKeys build the tracker key of a record per TrackBy.
var trackKeys = map[TrackBy]func(topic, stream string) string{
	TrackStream:      func(_, stream string) string { return stream },
	TrackTopicStream: func(topic, stream string) string { return topic + "|" + stream },
}

// TrackKey returns the key builder of a mode.
func (ExtractorManager) TrackKey(t TrackBy) (func(topic, stream string) string, error) {
	f, ok := trackKeys[t]
	if !ok {
		return nil, fmt.Errorf("unknown track-by %q (stream|topic-stream)", t)
	}
	return f, nil
}
