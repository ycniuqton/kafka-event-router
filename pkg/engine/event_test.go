package engine_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/eventrouter/router/pkg/engine"
)

func TestEventJSONCloneSnapshot(t *testing.T) {
	ev := engine.Event{Key: "k", Value: map[string]any{"a": []any{1.5, map[string]any{"b": "c"}}}, Headers: []engine.Header{{Key: "h", Value: []byte("v")}},
		Topic: "t", Partition: 2, Offset: 9, Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"key":"k","value":{"a":[1.5,{"b":"c"}]},"headers":[{"key":"h","value":"v"}],"topic":"t","partition":2,"offset":9,"timestamp":"2026-01-01T00:00:00Z"}` {
		t.Fatalf("json %s", raw)
	}
	var back engine.Event
	if err := json.Unmarshal(raw, &back); err != nil || !reflect.DeepEqual(back, ev) {
		t.Fatalf("round trip %+v %v", back, err)
	}
	c := ev.Clone()
	c.Value.(map[string]any)["a"].([]any)[1].(map[string]any)["b"] = "changed"
	c.Headers[0].Value[0] = 'x'
	if ev.Value.(map[string]any)["a"].([]any)[1].(map[string]any)["b"] != "c" || string(ev.Headers[0].Value) != "v" {
		t.Fatal("clone must be deep")
	}
	s := ev.Snapshot()
	if s["offset"] != int64(9) || s["timestamp"] != "2026-01-01T00:00:00Z" {
		t.Fatalf("snapshot %v", s)
	}
	upd := ev.WithSnapshot(map[string]any{"value": "v2", "headers": []any{map[string]any{"key": "x", "value": "y"}, "junk"}, "topic": "ignored"})
	if upd.Value != "v2" || upd.Key != "k" || upd.Topic != "t" || len(upd.Headers) != 1 || upd.Headers[0].Key != "x" {
		t.Fatalf("with snapshot %+v", upd)
	}
}
