package determinism

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestPinContextAndJSON(t *testing.T) {
	p := Current(1<<63+5, time.Date(2024, 1, 1, 0, 0, 0, 999999, time.UTC))
	if !p.Compatible() || p.Now.Nanosecond() != 0 {
		t.Fatalf("pin %+v", p)
	}
	got, ok := PinContext{}.From(PinContext{}.With(context.Background(), p))
	if !ok || got != p {
		t.Fatalf("context pin %+v %v", got, ok)
	}
	if _, ok := (PinContext{}).From(context.Background()); ok {
		t.Fatal("empty context must carry no pin")
	}
	raw, _ := json.Marshal(p)
	var back Pin
	if err := json.Unmarshal(raw, &back); err != nil || !back.Now.Equal(p.Now) || back.Seed != p.Seed {
		t.Fatalf("json %s -> %+v (%v)", raw, back, err)
	}
	if (Pin{Version: VersionCurrent + 1}).Compatible() {
		t.Fatal("a newer contract version is not compatible")
	}
}
