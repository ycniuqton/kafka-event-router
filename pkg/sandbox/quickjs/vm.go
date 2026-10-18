package quickjs

import (
	"context"
	"encoding/json"

	"github.com/eventrouter/router/pkg/sandbox"
	"github.com/eventrouter/router/pkg/sandbox/determinism"
)

// evalFlag is a QuickJS JS_Eval flag.
type evalFlag uint64

const (
	evalGlobal evalFlag = 0
	evalStrict evalFlag = 1 << 3
)

// guestGlobal names a global the host defines in the guest.
type guestGlobal string

// hostGlobal is the global under which the bridge function is handed to the prelude.
const hostGlobal guestGlobal = "__sb_host"

// guestVM is one booted QuickJS context: prelude installed, optionally a program loaded.
type guestVM struct {
	abi        *guestABI
	jsctx      guestPtr
	undefined  jsValue
	exports    [determinism.ExportCount]jsValue
	entry      jsValue
	argv       guestPtr
	classifier sandbox.ErrorClassifier
	position   stackPosition
}

// bootVM creates the QuickJS runtime, hands it the bridge and runs the prelude.
func bootVM(ctx context.Context, abi *guestABI, heapLimit int64, prelude determinism.Prelude) (*guestVM, error) {
	qjs, err := abi.call32(ctx, exNewQJS, uint64(heapLimit), 0, 0, uint64(8<<20))
	if err != nil || qjs == 0 {
		return nil, joinNil(err, errGuestBoot)
	}
	vm := &guestVM{abi: abi, classifier: sandbox.NewErrorClassifier()}
	if vm.jsctx, err = abi.call32(ctx, exGetContext, uint64(qjs)); err != nil {
		return nil, err
	}
	if vm.undefined, err = abi.call(ctx, exNewUndefined); err != nil {
		return nil, err
	}
	if vm.argv, err = abi.alloc(ctx, make([]byte, jsValueBytes), 0); err != nil {
		return nil, err
	}
	if err = vm.installBridge(ctx); err != nil {
		return nil, err
	}
	return vm, vm.runPrelude(ctx, prelude)
}

// installBridge exposes the host proxy as a global the prelude captures and deletes.
func (vm *guestVM) installBridge(ctx context.Context) error {
	proxy, err := vm.abi.call(ctx, exCreateFunctionProxy, uint64(vm.jsctx), 0, 0, 0)
	if err != nil {
		return err
	}
	if _, err = vm.checked(ctx, proxy); err != nil {
		return err
	}
	return vm.setGlobal(ctx, string(hostGlobal), proxy)
}

// setGlobal defines globalThis[name] = v (takes ownership of v).
func (vm *guestVM) setGlobal(ctx context.Context, name string, v jsValue) error {
	global, err := vm.abi.call(ctx, exGetGlobal, uint64(vm.jsctx))
	if err != nil {
		return err
	}
	namePtr, err := vm.abi.cstring(ctx, name)
	if err != nil {
		return err
	}
	_, err = vm.abi.call(ctx, exSetPropertyStr, uint64(vm.jsctx), global, uint64(namePtr), v)
	return joinNil(err, vm.abi.free(ctx, namePtr), vm.freeValue(ctx, global))
}

// runPrelude evaluates the prelude and keeps its exported functions.
func (vm *guestVM) runPrelude(ctx context.Context, prelude determinism.Prelude) error {
	arr, err := vm.evalSource(ctx, prelude.Source, prelude.FileName)
	if err != nil {
		return err
	}
	for i := range vm.exports {
		v, err := vm.abi.call(ctx, exGetPropertyUint32, uint64(vm.jsctx), arr, uint64(i))
		if err != nil {
			return err
		}
		vm.exports[i] = v
	}
	return vm.freeValue(ctx, arr)
}

// evalSource evaluates a script in the global scope (strict mode).
func (vm *guestVM) evalSource(ctx context.Context, src, file string) (jsValue, error) {
	code, err := vm.abi.cstring(ctx, src)
	if err != nil {
		return 0, err
	}
	defer vm.abi.free(ctx, code)
	return vm.eval(ctx, code, 0, 0, file)
}

func (vm *guestVM) eval(ctx context.Context, code, bytecode guestPtr, size uint32, file string) (jsValue, error) {
	opts, err := vm.evalOptions(ctx, code, bytecode, size, file)
	if err != nil {
		return 0, err
	}
	v, err := vm.abi.call(ctx, exEval, uint64(vm.jsctx), uint64(opts.opts))
	opts.release(ctx)
	if err != nil {
		return 0, err
	}
	return vm.checked(ctx, v)
}

// evalOptions allocates a QJSEvalOptions struct and its file name.
func (vm *guestVM) evalOptions(ctx context.Context, code, bytecode guestPtr, size uint32, file string) (evalOptions, error) {
	name, err := vm.abi.cstring(ctx, file)
	if err != nil {
		return evalOptions{}, err
	}
	flags := uint64(evalGlobal | evalStrict)
	opts, err := vm.abi.call32(ctx, exCreateEvalOption, uint64(code), uint64(bytecode), uint64(size), uint64(name), flags)
	if err != nil || opts == 0 {
		return evalOptions{}, joinNil(err, errGuestOutOfMemory)
	}
	return evalOptions{abi: vm.abi, opts: opts, name: name}, nil
}

// evalOptions owns the guest memory of one eval call.
type evalOptions struct {
	abi  *guestABI
	opts guestPtr
	name guestPtr
}

func (o evalOptions) release(ctx context.Context) {
	_ = o.abi.free(ctx, o.opts)
	_ = o.abi.free(ctx, o.name)
}

// compile returns QuickJS bytecode of src or a syntax error with position.
func (vm *guestVM) compile(ctx context.Context, src string) ([]byte, error) {
	code, err := vm.abi.cstring(ctx, src)
	if err != nil {
		return nil, err
	}
	defer vm.abi.free(ctx, code)
	opts, err := vm.evalOptions(ctx, code, 0, 0, string(userScript))
	if err != nil {
		return nil, err
	}
	cell, err := vm.abi.call32(ctx, exCompile, uint64(vm.jsctx), uint64(opts.opts))
	opts.release(ctx)
	if err != nil {
		return nil, err
	}
	if cell == 0 {
		return nil, vm.asCompileError(vm.exception(ctx))
	}
	return vm.takeBuffer(ctx, cell)
}

// takeBuffer copies a packed (addr, len) buffer out of the guest and frees it.
func (vm *guestVM) takeBuffer(ctx context.Context, cell guestPtr) ([]byte, error) {
	addr, n, err := vm.abi.unpack(ctx, cell)
	if err != nil {
		return nil, err
	}
	b, err := vm.abi.readBytes(addr, n)
	return b, joinNil(err, vm.abi.free(ctx, addr))
}

// loadProgram runs the user source and resolves the entry function. Source (not bytecode)
// is evaluated because the guest glue double-frees bytecode whose top level throws.
func (vm *guestVM) loadProgram(ctx context.Context, src guestSource) error {
	vm.position = src.position()
	v, err := vm.evalSource(ctx, src.code, string(userScript))
	if err != nil {
		return vm.asCompileError(err)
	}
	if err = vm.freeValue(ctx, v); err != nil {
		return err
	}
	if vm.entry, err = vm.evalSource(ctx, src.resolver(), string(resolverScript)); err != nil {
		return err
	}
	isFn, err := vm.abi.call32(ctx, exIsFunction, uint64(vm.jsctx), vm.entry)
	if err != nil {
		return err
	}
	if isFn == 0 {
		return vm.classifier.New(sandbox.ErrorCompile, string(sandbox.MsgTransformMissing))
	}
	return nil
}

// asCompileError re-labels a JS exception raised while loading as a compile error.
func (vm *guestVM) asCompileError(err error) error {
	if se, ok := sandbox.AsError(err); ok && se.Kind != sandbox.ErrorMemory {
		se.Kind, se.Retryable = sandbox.ErrorCompile, false
	}
	return err
}

// callExport calls prelude export idx with at most one argument.
func (vm *guestVM) callExport(ctx context.Context, idx determinism.PreludeExport, arg *jsValue) (jsValue, error) {
	argc := uint64(0)
	if arg != nil {
		vm.argvWith(*arg)
		argc = 1
	}
	v, err := vm.abi.call(ctx, exCall, uint64(vm.jsctx), vm.exports[idx], vm.undefined, argc, uint64(vm.argv))
	if err != nil {
		return 0, err
	}
	return vm.checked(ctx, v)
}

// invoke runs the entry function through the prelude and returns the envelope text.
func (vm *guestVM) invoke(ctx context.Context, limit int64) ([]byte, error) {
	v, err := vm.callExport(ctx, determinism.ExportInvoke, &vm.entry)
	if err != nil {
		return nil, err
	}
	raw, err := vm.abi.toString(ctx, vm.jsctx, v, limit)
	return raw, joinNil(err, vm.freeValue(ctx, v))
}

// lockdown freezes intrinsics for the lockdown_reuse policy.
func (vm *guestVM) lockdown(ctx context.Context) error {
	v, err := vm.callExport(ctx, determinism.ExportLockdown, nil)
	if err != nil {
		return err
	}
	return vm.freeValue(ctx, v)
}

// checked turns an exception result into a typed error.
func (vm *guestVM) checked(ctx context.Context, v jsValue) (jsValue, error) {
	isExc, err := vm.abi.call32(ctx, exIsException, v)
	if err != nil {
		return 0, err
	}
	if isExc != 0 {
		return 0, vm.exception(ctx)
	}
	return v, nil
}

// exception takes the pending exception and describes it via the prelude.
func (vm *guestVM) exception(ctx context.Context) error {
	exc, err := vm.abi.call(ctx, exGetException, uint64(vm.jsctx))
	if err != nil {
		return err
	}
	defer vm.freeValue(ctx, exc)
	if vm.exports[determinism.ExportDescribe] == 0 {
		raw, err := vm.abi.toString(ctx, vm.jsctx, exc, int64(sandbox.LimitDefault.MaxOutputBytes))
		return joinNil(err, vm.classifier.New(sandbox.ErrorInternal, string(raw)))
	}
	return vm.describe(ctx, exc)
}

// describe converts an exception value into a sandbox error.
func (vm *guestVM) describe(ctx context.Context, exc jsValue) error {
	d, err := vm.abi.call(ctx, exCall, uint64(vm.jsctx), vm.exports[determinism.ExportDescribe], vm.undefined, 1, uint64(vm.argvWith(exc)))
	if err != nil {
		return err
	}
	isExc, err := vm.abi.call32(ctx, exIsException, d)
	if err != nil || isExc != 0 {
		return joinNil(err, vm.classifier.New(sandbox.ErrorMemory, string(sandbox.MsgOutOfMemory)))
	}
	raw, err := vm.abi.toString(ctx, vm.jsctx, d, int64(sandbox.LimitDefault.MaxOutputBytes))
	if err = joinNil(err, vm.freeValue(ctx, d)); err != nil {
		return err
	}
	var g guestError
	if err := json.Unmarshal(raw, &g); err != nil {
		return vm.classifier.New(sandbox.ErrorInternal, err.Error())
	}
	return g.toError(vm.classifier, vm.position)
}

// argvWith stores v in the argv slot and returns the slot address.
func (vm *guestVM) argvWith(v jsValue) guestPtr {
	if mem, live := vm.abi.memory(); live {
		mem.WriteUint64Le(vm.argv, v)
	}
	return vm.argv
}

func (vm *guestVM) freeValue(ctx context.Context, v jsValue) error {
	_, err := vm.abi.call(ctx, exFreeValue, uint64(vm.jsctx), v)
	return err
}
