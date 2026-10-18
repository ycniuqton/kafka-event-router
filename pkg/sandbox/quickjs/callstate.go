package quickjs

import (
	"context"
	"encoding/json"
	"math/rand/v2"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/determinism"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// callState is the per-call data host functions and WASI stubs read from the call context.
type callState struct {
	abi      *guestABI
	input    []byte
	registry *host.Registry
	clock    determinism.FrozenClock
	random   *rand.Rand
	logs     *host.LogBuffer
	argLimit int64
}

type callStateKey struct{}

// withCallState attaches s to ctx for the duration of one guest call.
func withCallState(ctx context.Context, s *callState) context.Context {
	return context.WithValue(ctx, callStateKey{}, s)
}

// callStateFrom returns the state of the running call, or nil outside a call.
func callStateFrom(ctx context.Context) *callState {
	s, _ := ctx.Value(callStateKey{}).(*callState)
	return s
}

// guestInput is what the prelude's invoke() fetches through the "input" op.
type guestInput struct {
	Event     json.RawMessage         `json:"event"`
	NowMs     int64                   `json:"nowMs"`
	Predicate bool                    `json:"predicate"`
	Random    determinism.RandomState `json:"rng"`
}

// stateFactory builds call states for one instance.
type stateFactory struct {
	abi      *guestABI
	registry *host.Registry
	limits   sandbox.Limits
	seeds    determinism.SeedPolicy
}

// forCall builds the state of one invocation.
func (f stateFactory) forCall(call sandbox.GuestCall) (*callState, error) {
	input, err := json.Marshal(guestInput{
		Event:     call.Input,
		NowMs:     call.Clock.UnixMilli(),
		Predicate: call.Predicate,
		Random:    f.seeds.State(call.Seed),
	})
	if err != nil {
		return nil, err
	}
	return &callState{abi: f.abi, input: input, registry: f.registry, clock: call.Clock,
		random: rand.New(rand.NewPCG(call.Seed, call.Seed)), logs: call.Logs, argLimit: f.limits.MaxOutputBytes}, nil
}

// forLoad builds the state used while the prelude and top-level user code run.
func (f stateFactory) forLoad() *callState {
	return &callState{abi: f.abi, registry: f.registry, clock: determinism.Freeze(determinism.LoadEpoch),
		random: rand.New(rand.NewPCG(0, 0)), logs: host.NewLogBuffer(0), argLimit: f.limits.MaxOutputBytes}
}
