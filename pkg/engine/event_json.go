package engine

import (
	"encoding/json"
	"time"
)

// EventField names members of the event JSON shape (test_cases.input_event, JS boundary, trace).
type EventField string

const (
	FieldKey       EventField = "key"
	FieldValue     EventField = "value"
	FieldHeaders   EventField = "headers"
	FieldTopic     EventField = "topic"
	FieldPartition EventField = "partition"
	FieldOffset    EventField = "offset"
	FieldTimestamp EventField = "timestamp"
)

// EventDoc is the JSON shape of an Event.
type EventDoc struct {
	Key       any         `json:"key"`
	Value     any         `json:"value"`
	Headers   []HeaderDoc `json:"headers"`
	Topic     string      `json:"topic,omitempty"`
	Partition int32       `json:"partition"`
	Offset    int64       `json:"offset"`
	Timestamp *time.Time  `json:"timestamp,omitempty"`
}

// HeaderDoc is one header in the event JSON shape (value as UTF-8 text).
type HeaderDoc struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MarshalJSON writes the EventDoc shape.
func (e Event) MarshalJSON() ([]byte, error) {
	doc := EventDoc{Key: e.Key, Value: e.Value, Headers: []HeaderDoc{}, Topic: e.Topic, Partition: e.Partition, Offset: e.Offset}
	for _, h := range e.Headers {
		doc.Headers = append(doc.Headers, HeaderDoc{Key: h.Key, Value: string(h.Value)})
	}
	if !e.Timestamp.IsZero() {
		ts := e.Timestamp
		doc.Timestamp = &ts
	}
	return json.Marshal(doc)
}

// UnmarshalJSON reads the EventDoc shape.
func (e *Event) UnmarshalJSON(b []byte) error {
	var doc EventDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return err
	}
	*e = Event{Key: doc.Key, Value: doc.Value, Topic: doc.Topic, Partition: doc.Partition, Offset: doc.Offset}
	for _, h := range doc.Headers {
		e.Headers = append(e.Headers, Header{Key: h.Key, Value: []byte(h.Value)})
	}
	if doc.Timestamp != nil {
		e.Timestamp = *doc.Timestamp
	}
	return nil
}

// Clone returns a deep copy (used per branch and for trace snapshots).
func (e Event) Clone() Event {
	c := e
	c.Key = ValueCloner{}.Clone(e.Key)
	c.Value = ValueCloner{}.Clone(e.Value)
	c.Headers = nil
	for _, h := range e.Headers {
		c.Headers = append(c.Headers, Header{Key: h.Key, Value: append([]byte(nil), h.Value...)})
	}
	return c
}

// Snapshot returns the event as a generic JSON-shaped map; values are shared, not copied.
func (e Event) Snapshot() map[string]any {
	headers := make([]any, 0, len(e.Headers))
	for _, h := range e.Headers {
		headers = append(headers, map[string]any{string(FieldKey): h.Key, string(FieldValue): string(h.Value)})
	}
	return map[string]any{
		string(FieldKey):       e.Key,
		string(FieldValue):     e.Value,
		string(FieldHeaders):   headers,
		string(FieldTopic):     e.Topic,
		string(FieldPartition): e.Partition,
		string(FieldOffset):    e.Offset,
		string(FieldTimestamp): e.Timestamp.UTC().Format(time.RFC3339Nano),
	}
}

// WithSnapshot returns e with key, value and headers taken from a JSON-shaped map;
// members absent from m keep their current values and metadata is read-only.
func (e Event) WithSnapshot(m map[string]any) Event {
	out := e
	if v, ok := m[string(FieldKey)]; ok {
		out.Key = v
	}
	if v, ok := m[string(FieldValue)]; ok {
		out.Value = v
	}
	if v, ok := m[string(FieldHeaders)].([]any); ok {
		out.Headers = HeaderListReader{}.Read(v)
	}
	return out
}

// HeaderListReader converts the JSON header list back to Headers, skipping malformed entries.
type HeaderListReader struct{}

// Read converts entries of shape {key, value}.
func (HeaderListReader) Read(list []any) []Header {
	out := []Header{}
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		k, _ := m[string(FieldKey)].(string)
		v, _ := m[string(FieldValue)].(string)
		out = append(out, Header{Key: k, Value: []byte(v)})
	}
	return out
}

// ValueCloner deep-copies generic decoded values.
type ValueCloner struct{}

// Clone copies maps, slices and byte slices recursively; scalars are immutable.
func (c ValueCloner) Clone(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, e := range t {
			m[k] = c.Clone(e)
		}
		return m
	case []any:
		s := make([]any, len(t))
		for i, e := range t {
			s[i] = c.Clone(e)
		}
		return s
	case []byte:
		return append([]byte(nil), t...)
	}
	return v
}
