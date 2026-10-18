package routes

import (
	"encoding/json"
	"testing"
)

func TestDraftEditorDefaults(t *testing.T) {
	raw := json.RawMessage(`{"source":{"connectionId":"c1","topics":["t"]},"limits":{"maxOutputBytes":1048576}}`)
	out, err := DraftEditor{}.WithDefaults(raw, "Orders Feed")
	if err != nil {
		t.Fatal(err)
	}
	cfg := ConfigOf(out)
	if cfg == nil || cfg.Name != "Orders Feed" || *cfg.Source.ConsumerGroup != "router.orders-feed" || *cfg.Limits.MaxOutputBytes != 1048576 {
		t.Fatalf("%s", out)
	}
	kept, _ := DraftEditor{}.WithDefaults(json.RawMessage(`{"name":"x","source":{"consumerGroup":"g"}}`), "y")
	if c := ConfigOf(kept); c.Name != "x" || *c.Source.ConsumerGroup != "g" {
		t.Fatalf("%s", kept)
	}
	moved, _ := DraftEditor{}.Retarget(kept, "y-copy")
	if c := ConfigOf(moved); c.Name != "y-copy" || *c.Source.ConsumerGroup != "router.y-copy" {
		t.Fatalf("%s", moved)
	}
}

func TestDraftEditorIsObject(t *testing.T) {
	for raw, want := range map[string]bool{`{"a":1}`: true, ` {}`: true, `[]`: false, `"x"`: false, `{`: false, ``: false} {
		if got := (DraftEditor{}).IsObject(json.RawMessage(raw)); got != want {
			t.Errorf("%q: %v", raw, got)
		}
	}
}

func TestDraftEditorKeepsConsumerGroupOnSave(t *testing.T) {
	saved := json.RawMessage(`{"name":"x","source":{"connectionId":"c1","topics":["t"]}}`)
	out, _ := DraftEditor{}.KeepConsumerGroup(saved, json.RawMessage(`{"source":{"consumerGroup":"router.old"}}`), "renamed")
	if c := ConfigOf(out); c == nil || *c.Source.ConsumerGroup != "router.old" {
		t.Fatalf("previous group not kept: %s", out)
	}
	out, _ = DraftEditor{}.KeepConsumerGroup(saved, nil, "Orders Feed")
	if c := ConfigOf(out); *c.Source.ConsumerGroup != "router.orders-feed" {
		t.Fatalf("default group not applied: %s", out)
	}
	explicit, _ := DraftEditor{}.KeepConsumerGroup(json.RawMessage(`{"source":{"consumerGroup":"g"}}`), json.RawMessage(`{"source":{"consumerGroup":"router.old"}}`), "x")
	if c := ConfigOf(explicit); *c.Source.ConsumerGroup != "g" {
		t.Fatalf("explicit group overwritten: %s", explicit)
	}
}
