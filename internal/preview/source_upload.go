package preview

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/kafka"
)

// UploadSource builds a session from uploaded events (JSON array or NDJSON); no connection needed.
type UploadSource struct {
	Clock func() time.Time
}

func (UploadSource) Mode() SampleMode { return ModeUpload }

// Load parses the upload, validates each item and admits records until a limit is hit.
func (s UploadSource) Load(_ context.Context, req LoadRequest, budget *kafka.SamplingBudget) (LoadResult, error) {
	events, field, err := s.events(req)
	if err != nil {
		return LoadResult{}, err
	}
	if len(events) == 0 {
		return LoadResult{}, ErrUploadEmpty.At(string(field))
	}
	conv := InlineEventConverter{Format: req.EffectiveFormat(), Topic: UploadTopicOf(req), Now: s.Clock()}
	res := LoadResult{Records: []SampleRecord{}}
	for i, ev := range events {
		rec, err := conv.Record(i, ev)
		if err != nil {
			return LoadResult{}, ErrUploadItem.At(field.Indexed(i)).Wrap(err)
		}
		if !budget.Admit(rec.Size(), s.Clock()) {
			break
		}
		res.Records = append(res.Records, rec)
		res.Bytes += rec.Size()
	}
	res.StopReason = budget.Reason()
	return res, nil
}

// events returns the structured events or the parsed upload text, with the field they came from.
func (s UploadSource) events(req LoadRequest) ([]InlineEvent, RequestField, error) {
	if strings.TrimSpace(req.Upload) == "" {
		return req.Events, FieldEvents, nil
	}
	events, err := UploadParser{}.Parse(req.Upload)
	return events, FieldUpload, err
}

// UploadTopicOf returns the first requested topic, or the synthetic upload topic.
func UploadTopicOf(req LoadRequest) string {
	if len(req.Topics) > 0 {
		return req.Topics[0]
	}
	return string(UploadTopic)
}

// UploadParser reads upload text: a JSON array, or one JSON value per line (NDJSON).
type UploadParser struct{}

// ArrayOpen is the first byte of a JSON array upload.
const ArrayOpen = '['

// Parse returns the events of text; items without the event shape become the event value.
func (p UploadParser) Parse(text string) ([]InlineEvent, error) {
	trimmed := bytes.TrimSpace([]byte(text))
	if len(trimmed) > 0 && trimmed[0] == ArrayOpen {
		return p.array(trimmed)
	}
	return p.lines(trimmed)
}

func (p UploadParser) array(raw []byte) ([]InlineEvent, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, problem.NewError(KindInvalidUpload, fmt.Sprintf("The file is not a valid JSON array: %v.", err)).At(string(FieldUpload))
	}
	out := make([]InlineEvent, 0, len(items))
	for i, item := range items {
		ev, err := EventShape{}.Read(item)
		if err != nil {
			return nil, ErrUploadItem.At(FieldUpload.Indexed(i)).Wrap(err)
		}
		out = append(out, ev)
	}
	return out, nil
}

func (p UploadParser) lines(raw []byte) ([]InlineEvent, error) {
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, int(UploadLineBuffer)), int(MaxBytes))
	out := []InlineEvent{}
	for line := 1; sc.Scan(); line++ {
		text := bytes.TrimSpace(sc.Bytes())
		if len(text) == 0 {
			continue
		}
		ev, err := EventShape{}.Read(append([]byte(nil), text...))
		if err != nil {
			return nil, problem.NewError(KindInvalidUpload, fmt.Sprintf("Line %d is not valid JSON.", line)).At(string(FieldUpload)).Wrap(err)
		}
		out = append(out, ev)
	}
	return out, sc.Err()
}

// UploadBuffer sizes the NDJSON line scanner's initial buffer.
type UploadBuffer int

const UploadLineBuffer UploadBuffer = 64 << 10

// EventShape recognises the event JSON shape ({key, value, headers, timestamp, …}).
type EventShape struct{}

// eventMembers are the members an event-shaped object may have.
var eventMembers = map[engine.EventField]bool{
	engine.FieldKey: true, engine.FieldValue: true, engine.FieldHeaders: true, engine.FieldTimestamp: true,
	engine.FieldTopic: true, engine.FieldPartition: true, engine.FieldOffset: true,
}

// Read decodes item as an event when it has the event shape, else as a bare value.
func (EventShape) Read(item json.RawMessage) (InlineEvent, error) {
	if !json.Valid(item) {
		return InlineEvent{}, fmt.Errorf("invalid JSON")
	}
	var members map[string]json.RawMessage
	if json.Unmarshal(item, &members) != nil || !(EventShape{}).matches(members) {
		return InlineEvent{Value: item}, nil
	}
	var ev InlineEvent
	if err := json.Unmarshal(item, &ev); err != nil {
		return InlineEvent{}, err
	}
	return ev, nil
}

func (EventShape) matches(members map[string]json.RawMessage) bool {
	if _, ok := members[string(engine.FieldValue)]; !ok {
		return false
	}
	for k := range members {
		if !eventMembers[engine.EventField(k)] {
			return false
		}
	}
	return true
}

// InlineEventConverter turns request events into session records for a value format.
type InlineEventConverter struct {
	Format config.FormatType
	Topic  string
	Now    time.Time
}

// Record converts event i; a missing value is an error.
func (c InlineEventConverter) Record(i int, ev InlineEvent) (SampleRecord, error) {
	if len(ev.Value) == 0 || !json.Valid(ev.Value) {
		return SampleRecord{}, fmt.Errorf("event %d has no valid value", i)
	}
	rec := SampleRecord{Seq: i, Topic: c.Topic, Partition: int32(SyntheticPartition), Offset: int64(i), Timestamp: c.Now, Headers: []engine.Header{}}
	rec.Key = JSONBytes{}.Key(ev.Key)
	rec.Value = JSONBytes{}.Value(ev.Value, c.Format)
	for _, h := range ev.Headers {
		rec.Headers = append(rec.Headers, engine.Header{Key: h.Key, Value: []byte(h.Value)})
	}
	c.coordinates(&rec, ev)
	return rec, nil
}

func (c InlineEventConverter) coordinates(rec *SampleRecord, ev InlineEvent) {
	if ev.Timestamp != nil {
		rec.Timestamp = ev.Timestamp.UTC()
	}
	if ev.Topic != "" {
		rec.Topic = ev.Topic
	}
	if ev.Partition != nil {
		rec.Partition = *ev.Partition
	}
	if ev.Offset != nil {
		rec.Offset = *ev.Offset
	}
}

// JSONBytes renders request JSON members as record bytes.
type JSONBytes struct{}

// Key returns nil for absent/null keys, the text of string keys, else the JSON text.
func (JSONBytes) Key(raw json.RawMessage) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == string(JSONNull) {
		return nil
	}
	var s string
	if json.Unmarshal(trimmed, &s) == nil {
		return []byte(s)
	}
	return append([]byte(nil), trimmed...)
}

// Value returns the JSON text for JSON formats, the text of a string for other formats.
func (JSONBytes) Value(raw json.RawMessage, format config.FormatType) []byte {
	trimmed := bytes.TrimSpace(raw)
	if format == config.FormatTypeJSON {
		return append([]byte(nil), trimmed...)
	}
	var s string
	if json.Unmarshal(trimmed, &s) == nil {
		return []byte(s)
	}
	return append([]byte(nil), trimmed...)
}
