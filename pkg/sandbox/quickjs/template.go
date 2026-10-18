package quickjs

import (
	"context"

	"github.com/tetratelabs/wazero/api"

	"github.com/eventrouter/router/pkg/sandbox/determinism"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// bootTemplate is the memory image of a guest right after QuickJS and the prelude booted.
// Cloning it into a fresh module skips the costly boot; every handle keeps its address.
type bootTemplate struct {
	image []byte
	vm    guestVM
}

// buildTemplate boots one scratch guest and captures its image.
func (g *engineGroup) buildTemplate(ctx context.Context, prelude determinism.Prelude, heapLimit int64) (*bootTemplate, error) {
	mod, err := g.newModule(ctx)
	if err != nil {
		return nil, err
	}
	defer mod.Close(ctx)
	abi, err := newGuestABI(mod)
	if err != nil {
		return nil, err
	}
	bootCtx := withCallState(ctx, stateFactory{abi: abi, registry: host.NewRegistry()}.forLoad())
	vm, err := bootVM(bootCtx, abi, heapLimit, prelude)
	if err != nil {
		return nil, err
	}
	image, err := abi.readBytes(0, mod.Memory().Size())
	if err != nil {
		return nil, err
	}
	return &bootTemplate{image: image, vm: *vm}, nil
}

// template returns the group's boot template, building it on first use.
func (g *engineGroup) template(ctx context.Context, prelude determinism.Prelude, heapLimit int64) (*bootTemplate, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.boot != nil {
		return g.boot, nil
	}
	t, err := g.buildTemplate(ctx, prelude, heapLimit)
	if err != nil {
		return nil, err
	}
	g.boot = t
	return t, nil
}

// clone writes the template image into mod and returns a VM bound to it.
func (t *bootTemplate) clone(mod api.Module) (*guestVM, error) {
	mem := mod.Memory()
	if need := uint32(len(t.image)); mem.Size() < need {
		if _, ok := mem.Grow((need - mem.Size()) / uint32(wasmPageBytes)); !ok {
			return nil, errGuestOutOfMemory
		}
	}
	if !mem.Write(0, t.image) {
		return nil, errGuestOutOfBounds
	}
	abi, err := newGuestABI(mod)
	if err != nil {
		return nil, err
	}
	vm := t.vm
	vm.abi = abi
	return &vm, nil
}
