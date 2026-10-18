package determinism

import (
	"context"
	"time"
)

// Pin is the determinism input stored with a saved test case: the contract version it was
// recorded under, the Math.random seed and the frozen ctx.now() of the recorded run.
type Pin struct {
	Version Version   `json:"determinismVersion"`
	Seed    uint64    `json:"seed,string"`
	Now     time.Time `json:"now"`
}

// Current pins seed and now under the running contract version (used when a case is saved).
func Current(seed uint64, now time.Time) Pin {
	return Pin{Version: VersionCurrent, Seed: seed, Now: Freeze(now).Now()}
}

// Compatible reports whether expectations recorded under p are valid for the running contract.
func (p Pin) Compatible() bool {
	return p.Version == VersionCurrent
}

// pinKey is the context key of a Pin.
type pinKey struct{}

// PinContext carries a Pin through callers that only pass a context (the engine's script port).
type PinContext struct{}

// With returns ctx carrying p.
func (PinContext) With(ctx context.Context, p Pin) context.Context {
	return context.WithValue(ctx, pinKey{}, p)
}

// From returns the Pin carried by ctx.
func (PinContext) From(ctx context.Context) (Pin, bool) {
	p, ok := ctx.Value(pinKey{}).(Pin)
	return p, ok
}
