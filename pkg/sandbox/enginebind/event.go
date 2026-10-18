package enginebind

import (
	"time"

	"github.com/eventrouter/router/pkg/sandbox/boundary"
)

// SnapshotField names members of the engine's event snapshot map.
type SnapshotField string

const (
	snapKey       SnapshotField = "key"
	snapValue     SnapshotField = "value"
	snapHeaders   SnapshotField = "headers"
	snapTopic     SnapshotField = "topic"
	snapPartition SnapshotField = "partition"
	snapOffset    SnapshotField = "offset"
	snapTimestamp SnapshotField = "timestamp"
)

// SnapshotCodec converts between engine event snapshots and boundary events.
type SnapshotCodec struct{}

// ToGuest reads an engine snapshot (Event.Snapshot shape).
func (c SnapshotCodec) ToGuest(m map[string]any) boundary.GuestEvent {
	ev := boundary.GuestEvent{Key: m[string(snapKey)], Value: m[string(snapValue)]}
	ev.Topic, _ = m[string(snapTopic)].(string)
	ev.Partition = int32(Number{}.Int64(m[string(snapPartition)]))
	ev.Offset = Number{}.Int64(m[string(snapOffset)])
	if ts, ok := m[string(snapTimestamp)].(string); ok {
		ev.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
	}
	ev.Headers = c.headers(m[string(snapHeaders)])
	return ev
}

func (SnapshotCodec) headers(v any) []boundary.Header {
	list, _ := v.([]any)
	out := make([]boundary.Header, 0, len(list))
	for _, item := range list {
		h, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key, _ := h[string(snapKey)].(string)
		val, _ := h[string(snapValue)].(string)
		out = append(out, boundary.Header{Key: key, Value: []byte(val)})
	}
	return out
}

// FromGuest renders one output in the snapshot shape read by Event.WithSnapshot.
func (SnapshotCodec) FromGuest(o boundary.Output) map[string]any {
	m := map[string]any{string(snapValue): o.Value}
	if o.KeySet {
		m[string(snapKey)] = o.Key
	}
	if o.HeadersSet {
		list := make([]any, 0, len(o.Headers))
		for _, h := range o.Headers {
			list = append(list, map[string]any{string(snapKey): h.Key, string(snapValue): string(h.Value)})
		}
		m[string(snapHeaders)] = list
	}
	return m
}

// Number reads integer-like snapshot members.
type Number struct{}

// Int64 accepts Go integer and float kinds; anything else is 0.
func (Number) Int64(v any) int64 {
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}
