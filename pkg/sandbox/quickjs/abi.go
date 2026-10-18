package quickjs

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// jsValue is a NaN-boxed QuickJS JSValue (64 bit on wasm32).
type jsValue = uint64

// guestPtr is an address in guest linear memory.
type guestPtr = uint32

// guestExport names a function exported by the QuickJS guest.
type guestExport string

const (
	exMalloc              guestExport = "malloc"
	exFree                guestExport = "free"
	exNewQJS              guestExport = "New_QJS"
	exGetContext          guestExport = "QJS_GetContext"
	exCreateEvalOption    guestExport = "QJS_CreateEvalOption"
	exEval                guestExport = "QJS_Eval"
	exCompile             guestExport = "QJS_Compile2"
	exIsException         guestExport = "QJS_IsException"
	exIsFunction          guestExport = "QJS_IsFunction"
	exGetException        guestExport = "JS_GetException"
	exToCString           guestExport = "QJS_ToCString"
	exFreeCString         guestExport = "JS_FreeCString"
	exFreeValue           guestExport = "JS_FreeValue"
	exNewString           guestExport = "QJS_NewString"
	exNewUndefined        guestExport = "JS_NewUndefined"
	exGetGlobal           guestExport = "JS_GetGlobalObject"
	exSetPropertyStr      guestExport = "JS_SetPropertyStr"
	exGetPropertyUint32   guestExport = "QJS_GetPropertyUint32"
	exCall                guestExport = "QJS_Call"
	exCreateFunctionProxy guestExport = "QJS_CreateFunctionProxy"
)

var requiredExports = []guestExport{
	exMalloc, exFree, exNewQJS, exGetContext, exCreateEvalOption, exEval, exCompile, exIsException,
	exIsFunction, exGetException, exToCString, exFreeCString, exFreeValue, exNewString, exNewUndefined,
	exGetGlobal, exSetPropertyStr, exGetPropertyUint32, exCall, exCreateFunctionProxy,
}

// abiLayout holds sizes of guest-side C structures.
type abiLayout uint32

const (
	packedPtrBytes abiLayout = 8
	jsValueBytes   abiLayout = 8
	cStringNUL     abiLayout = 1
	packedAddrBits abiLayout = 32
)

// guestABI wraps the raw exports of one guest module instance.
type guestABI struct {
	mod api.Module
	fns map[guestExport]api.Function
}

// newGuestABI resolves every export the engine relies on.
func newGuestABI(mod api.Module) (*guestABI, error) {
	g := &guestABI{mod: mod, fns: map[guestExport]api.Function{}}
	for _, name := range requiredExports {
		fn := mod.ExportedFunction(string(name))
		if fn == nil {
			return nil, fmt.Errorf("quickjs guest does not export %s", name)
		}
		g.fns[name] = fn
	}
	return g, nil
}

// call invokes an export and returns its first result (0 when void).
func (g *guestABI) call(ctx context.Context, name guestExport, args ...uint64) (uint64, error) {
	res, err := g.fns[name].Call(ctx, args...)
	if err != nil || len(res) == 0 {
		return 0, err
	}
	return res[0], nil
}

// call32 invokes an export returning an i32 (pointer or C bool).
func (g *guestABI) call32(ctx context.Context, name guestExport, args ...uint64) (uint32, error) {
	v, err := g.call(ctx, name, args...)
	return api.DecodeU32(v), err
}

// memory returns the guest linear memory unless the module was closed: a timeout closes the
// module asynchronously and the next guest call unmaps its memory, so host code must never
// touch the memory of a closed module.
func (g *guestABI) memory() (api.Memory, bool) {
	if g.mod.IsClosed() {
		return nil, false
	}
	return g.mod.Memory(), true
}

// alloc copies b (plus nul bytes) into fresh guest memory.
func (g *guestABI) alloc(ctx context.Context, b []byte, nul abiLayout) (guestPtr, error) {
	ptr, err := g.call32(ctx, exMalloc, uint64(len(b))+uint64(nul))
	if err != nil {
		return 0, err
	}
	mem, live := g.memory()
	if !live {
		return 0, errInstanceClosed
	}
	if ptr == 0 || !mem.Write(ptr, b) {
		return 0, errGuestOutOfMemory
	}
	if nul > 0 && !mem.WriteByte(ptr+uint32(len(b)), 0) {
		return 0, errGuestOutOfMemory
	}
	return ptr, nil
}

// cstring copies s as a NUL-terminated C string.
func (g *guestABI) cstring(ctx context.Context, s string) (guestPtr, error) {
	return g.alloc(ctx, []byte(s), cStringNUL)
}

// free releases guest memory from malloc.
func (g *guestABI) free(ctx context.Context, ptr guestPtr) error {
	_, err := g.call(ctx, exFree, uint64(ptr))
	return err
}

// unpack reads a heap cell holding (addr << 32 | len) and frees the cell.
func (g *guestABI) unpack(ctx context.Context, cell guestPtr) (guestPtr, uint32, error) {
	raw, err := g.view(cell, uint32(packedPtrBytes))
	if err != nil {
		return 0, 0, err
	}
	packed := binary.LittleEndian.Uint64(raw)
	return guestPtr(packed >> packedAddrBits), uint32(packed), g.free(ctx, cell)
}

// view returns n bytes at ptr without copying; use it before the next guest call only.
func (g *guestABI) view(ptr guestPtr, n uint32) ([]byte, error) {
	mem, live := g.memory()
	if !live {
		return nil, errInstanceClosed
	}
	raw, ok := mem.Read(ptr, n)
	if !ok {
		return nil, errGuestOutOfBounds
	}
	return raw, nil
}

// readBytes copies n bytes at ptr out of guest memory.
func (g *guestABI) readBytes(ptr guestPtr, n uint32) ([]byte, error) {
	view, err := g.view(ptr, n)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), view...), nil
}

// toString converts a JS value to a Go string, refusing strings longer than limit.
func (g *guestABI) toString(ctx context.Context, jsctx guestPtr, v jsValue, limit int64) ([]byte, error) {
	cell, err := g.call32(ctx, exToCString, uint64(jsctx), v)
	if err != nil || cell == 0 {
		return nil, joinNil(err, errGuestNotString)
	}
	addr, n, err := g.unpack(ctx, cell)
	if err != nil {
		return nil, err
	}
	defer g.call(ctx, exFreeCString, uint64(jsctx), uint64(addr))
	if int64(n) > limit {
		return nil, &tooLargeError{size: int64(n)}
	}
	return g.readBytes(addr, n)
}

// newString creates a JS string value from s.
func (g *guestABI) newString(ctx context.Context, jsctx guestPtr, s []byte) (jsValue, error) {
	ptr, err := g.alloc(ctx, s, cStringNUL)
	if err != nil {
		return 0, err
	}
	v, err := g.call(ctx, exNewString, uint64(jsctx), uint64(ptr))
	return v, joinNil(err, g.free(ctx, ptr))
}

// memoryBytes returns the current size of linear memory.
func (g *guestABI) memoryBytes() int64 {
	return int64(g.mod.Memory().Size())
}
