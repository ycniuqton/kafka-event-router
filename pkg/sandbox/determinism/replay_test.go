package determinism

import (
	"context"
	"testing"
	"time"
)

func TestProcessingPolicySameSeedForReplay(t *testing.T) {
	p := ProcessingPolicy{}
	ref := EventRef{RouteID: "r1", Topic: "orders", Partition: 2, Offset: 99}
	first := p.Inputs(ref, "", time.Unix(100, 0))
	replay := p.Inputs(ref, PassReplay, time.Unix(500, 0))
	if first.Seed != replay.Seed || first.Seed != (SeedPolicy{}).Seed("r1", "orders", 2, 99) {
		t.Fatalf("replay seed %#x differs from original %#x", replay.Seed, first.Seed)
	}
	if first.Pass != PassOriginal || replay.Pass != PassReplay || !replay.Now.Equal(time.Unix(500, 0)) {
		t.Fatalf("inputs %+v / %+v", first, replay)
	}
}

func TestPassContext(t *testing.T) {
	ctx := context.Background()
	if (PassContext{}).From(ctx) != PassOriginal {
		t.Fatal("default pass is original")
	}
	if (PassContext{}).From((PassContext{}).With(ctx, PassReplay)) != PassReplay {
		t.Fatal("replay pass must round-trip")
	}
}
