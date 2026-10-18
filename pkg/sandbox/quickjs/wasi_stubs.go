package quickjs

import (
	"context"
	"encoding/binary"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"

	"github.com/eventrouter/router/pkg/sandbox/determinism"
)

// wasiFn names a WASI preview1 function with a non-default stub.
type wasiFn string

const (
	wasiClockTimeGet    wasiFn = "clock_time_get"
	wasiRandomGet       wasiFn = "random_get"
	wasiFdWrite         wasiFn = "fd_write"
	wasiProcExit        wasiFn = "proc_exit"
	wasiArgsSizesGet    wasiFn = "args_sizes_get"
	wasiEnvironSizesGet wasiFn = "environ_sizes_get"
	wasiArgsGet         wasiFn = "args_get"
	wasiEnvironGet      wasiFn = "environ_get"
	wasiFdPrestatGet    wasiFn = "fd_prestat_get"
	wasiFdFdstatGet     wasiFn = "fd_fdstat_get"
)

// wasiErrno is a WASI errno value.
type wasiErrno uint32

const (
	errnoSuccess wasiErrno = 0
	errnoBadf    wasiErrno = 8
	errnoNosys   wasiErrno = 52
)

// iovecLayout is the guest layout of a WASI ciovec {ptr u32, len u32}.
type iovecLayout uint32

const (
	iovecSize      iovecLayout = 8
	iovecLenOffset iovecLayout = 4
)

// wasiStubs is the deterministic WASI module: frozen clock, seeded random, discarded output,
// no args, env, files or sockets (every unlisted function returns ENOSYS).
type wasiStubs struct{}

// instantiate exports a stub for every WASI function the guest imports.
func (w wasiStubs) instantiate(ctx context.Context, rt wazero.Runtime, guest wazero.CompiledModule) error {
	b := rt.NewHostModuleBuilder(string(moduleWASI))
	handlers := w.handlers()
	for _, f := range guest.ImportedFunctions() {
		mod, name, _ := f.Import()
		if hostModule(mod) != moduleWASI {
			continue
		}
		h, ok := handlers[wasiFn(name)]
		if !ok {
			h = w.errno(errnoNosys, len(f.ResultTypes()) > 0)
		}
		b.NewFunctionBuilder().WithGoModuleFunction(h, f.ParamTypes(), f.ResultTypes()).Export(name)
	}
	_, err := b.Instantiate(ctx)
	return err
}

func (w wasiStubs) handlers() map[wasiFn]api.GoModuleFunc {
	return map[wasiFn]api.GoModuleFunc{
		wasiClockTimeGet:    w.clockTimeGet,
		wasiRandomGet:       w.randomGet,
		wasiFdWrite:         w.fdWrite,
		wasiProcExit:        w.procExit,
		wasiArgsSizesGet:    w.zeroSizes,
		wasiEnvironSizesGet: w.zeroSizes,
		wasiArgsGet:         w.errno(errnoSuccess, true),
		wasiEnvironGet:      w.errno(errnoSuccess, true),
		wasiFdPrestatGet:    w.errno(errnoBadf, true),
		wasiFdFdstatGet:     w.errno(errnoBadf, true),
	}
}

// errno returns a stub that only reports code.
func (wasiStubs) errno(code wasiErrno, hasResult bool) api.GoModuleFunc {
	return func(_ context.Context, _ api.Module, stack []uint64) {
		if hasResult {
			stack[0] = uint64(code)
		}
	}
}

// clockTimeGet(id, precision, resultPtr) writes the frozen clock of the call.
func (wasiStubs) clockTimeGet(ctx context.Context, mod api.Module, stack []uint64) {
	clock := determinism.Freeze(determinism.LoadEpoch)
	if s := callStateFrom(ctx); s != nil {
		clock = s.clock
	}
	mod.Memory().WriteUint64Le(api.DecodeU32(stack[2]), uint64(clock.UnixNano()))
	stack[0] = uint64(errnoSuccess)
}

// randomGet(buf, len) fills buf from the per-call seeded generator.
func (wasiStubs) randomGet(ctx context.Context, mod api.Module, stack []uint64) {
	buf, ok := mod.Memory().Read(api.DecodeU32(stack[0]), api.DecodeU32(stack[1]))
	s := callStateFrom(ctx)
	if !ok || s == nil {
		stack[0] = uint64(errnoBadf)
		return
	}
	for i := range buf {
		buf[i] = byte(s.random.Uint32())
	}
	stack[0] = uint64(errnoSuccess)
}

// fdWrite(fd, iovs, iovsLen, nwrittenPtr) discards output but reports it as written.
func (wasiStubs) fdWrite(_ context.Context, mod api.Module, stack []uint64) {
	iovs, count := api.DecodeU32(stack[1]), api.DecodeU32(stack[2])
	raw, ok := mod.Memory().Read(iovs, count*uint32(iovecSize))
	if !ok {
		stack[0] = uint64(errnoBadf)
		return
	}
	var total uint32
	for i := uint32(0); i < count; i++ {
		total += binary.LittleEndian.Uint32(raw[i*uint32(iovecSize)+uint32(iovecLenOffset):])
	}
	mod.Memory().WriteUint32Le(api.DecodeU32(stack[3]), total)
	stack[0] = uint64(errnoSuccess)
}

// procExit traps the guest; the instance is discarded by the pool.
func (wasiStubs) procExit(_ context.Context, _ api.Module, stack []uint64) {
	panic(sys.NewExitError(api.DecodeU32(stack[0])))
}

// zeroSizes reports zero args/env entries.
func (wasiStubs) zeroSizes(_ context.Context, mod api.Module, stack []uint64) {
	mod.Memory().WriteUint32Le(api.DecodeU32(stack[0]), 0)
	mod.Memory().WriteUint32Le(api.DecodeU32(stack[1]), 0)
	stack[0] = uint64(errnoSuccess)
}
