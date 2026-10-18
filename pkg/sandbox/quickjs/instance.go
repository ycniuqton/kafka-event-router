package quickjs

import (
	"context"

	"github.com/tetratelabs/wazero/api"

	"github.com/eventrouter/router/pkg/sandbox"
)

// instance is one wazero module instance running one QuickJS runtime.
type instance struct {
	mod      api.Module
	vm       *guestVM
	states   stateFactory
	traps    trapClassifier
	decoder  replyDecoder
	limits   sandbox.Limits
	snapshot *memorySnapshot
	reusable bool
	closed   bool
}

// Invoke resets guest state, runs the entry under the call deadline and decodes the reply.
func (i *instance) Invoke(ctx context.Context, call sandbox.GuestCall) (sandbox.GuestReply, error) {
	if i.closed || !i.reset() {
		i.reusable = false
		return sandbox.GuestReply{}, i.traps.classify(ctx, errInstanceClosed)
	}
	state, err := i.states.forCall(call)
	if err != nil {
		return sandbox.GuestReply{}, i.traps.classifier.New(sandbox.ErrorInternal, err.Error())
	}
	callCtx, cancel := context.WithTimeout(ctx, i.limits.Timeout)
	defer cancel()
	raw, err := i.vm.invoke(withCallState(callCtx, state), i.limits.MaxOutputBytes+int64(envelopeBytes))
	if err != nil {
		return sandbox.GuestReply{}, i.fail(callCtx, err)
	}
	if i.snapshot != nil && i.snapshot.grew(i.mod.Memory()) {
		i.reusable = false
	}
	return i.decoder.decode(raw)
}

// reset restores the pristine image under fresh_context; lockdown_reuse keeps state.
func (i *instance) reset() bool {
	if i.snapshot == nil {
		return true
	}
	return i.snapshot.restore(i.mod.Memory())
}

// fail classifies an error; anything but a JS-level error poisons the instance.
func (i *instance) fail(callCtx context.Context, err error) error {
	if se, ok := sandbox.AsError(err); ok {
		if i.traps.classifier.Policy(se.Kind).Action == sandbox.PoolDiscard {
			i.reusable = false
		}
		return se
	}
	i.reusable = false
	return i.traps.classify(callCtx, err)
}

// MemoryBytes is the guest linear memory plus the Go-side snapshot.
func (i *instance) MemoryBytes() int64 {
	if i.closed {
		return 0
	}
	total := i.vm.abi.memoryBytes()
	if i.snapshot != nil {
		total += i.snapshot.bytes()
	}
	return total
}

// Reusable is false after a trap, a timeout, OOM or a memory grow.
func (i *instance) Reusable() bool { return i.reusable && !i.closed }

// Close releases the module and its memory.
func (i *instance) Close(ctx context.Context) error {
	if i.closed {
		return nil
	}
	i.closed = true
	return i.mod.Close(ctx)
}
