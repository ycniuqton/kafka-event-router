package quickjs

import (
	"context"
	"sync"

	"github.com/tetratelabs/wazero/api"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// moduleHandle is a guest module instance.
type moduleHandle = api.Module

// compilerVM is a shared guest used only to syntax-check sources; its state is
// restored after every compile and it is rebuilt after a trap.
type compilerVM struct {
	rt       *Runtime
	mu       sync.Mutex
	mod      api.Module
	vm       *guestVM
	snapshot *memorySnapshot
}

// compile syntax-checks gs, returning a compile error with position.
func (c *compilerVM) compile(ctx context.Context, gs guestSource) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensure(ctx); err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, instanceLoadTimeout.duration())
	defer cancel()
	c.vm.position = gs.position()
	_, err := c.vm.compile(callCtx, gs.code)
	if _, ok := sandbox.AsError(err); err != nil && !ok {
		c.drop(ctx)
		return trapClassifier{classifier: c.rt.classifier, limits: c.rt.opts.CheckLimits}.classify(callCtx, err)
	}
	if !c.snapshot.restore(c.mod.Memory()) || c.snapshot.grew(c.mod.Memory()) {
		c.drop(ctx)
	}
	return err
}

// ensure boots the compiler guest on first use or after a drop.
func (c *compilerVM) ensure(ctx context.Context) error {
	if c.vm != nil {
		return nil
	}
	lim := c.rt.opts.CheckLimits
	grp, err := c.rt.group(ctx, lim.MemoryBytes)
	if err != nil {
		return err
	}
	mod, err := grp.newModule(ctx)
	if err != nil {
		return err
	}
	tpl, err := grp.template(ctx, c.rt.opts.Prelude, budgetFor(lim.MemoryBytes).heapLimit)
	if err == nil {
		c.vm, err = tpl.clone(mod)
	}
	if err == nil {
		bootCtx := withCallState(ctx, stateFactory{abi: c.vm.abi, registry: host.NewRegistry(), limits: lim}.forLoad())
		c.snapshot, err = captureSnapshot(bootCtx, c.vm.abi, budgetFor(lim.MemoryBytes).headroom)
	}
	if err != nil {
		c.vm = nil
		_ = mod.Close(ctx)
		return err
	}
	c.mod = mod
	return nil
}

// drop discards the compiler guest; the next compile boots a new one.
func (c *compilerVM) drop(ctx context.Context) {
	if c.mod != nil {
		_ = c.mod.Close(ctx)
	}
	c.mod, c.vm, c.snapshot = nil, nil, nil
}
