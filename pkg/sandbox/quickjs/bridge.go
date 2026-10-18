package quickjs

import (
	"context"
	"encoding/binary"
	"encoding/json"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/eventrouter/router/pkg/sandbox/host"
)

// hostModule names an import module of the guest.
type hostModule string

const (
	moduleEnv  hostModule = "env"
	moduleWASI hostModule = "wasi_snapshot_preview1"
)

// hostImport names a function imported by the guest.
type hostImport string

// importProxy is the single host function QuickJS calls for every proxied JS function.
const importProxy hostImport = "jsFunctionProxy"

// proxyArg is a position in the argv the guest proxy passes (handler, ctx, async, promise, ...user).
type proxyArg uint32

const (
	proxyArgOp      proxyArg = 4
	proxyArgName    proxyArg = 5
	proxyArgPayload proxyArg = 6
	proxyArgCount   proxyArg = 7
)

// guestOp is a request the prelude sends through the bridge.
type guestOp string

const (
	opInput guestOp = "input"
	opHost  guestOp = "host"
)

// bridgeLimit bounds the op and helper-name strings read from the guest.
type bridgeLimit int64

const bridgeNameLimit bridgeLimit = 256

// bridgeReply is the envelope returned to the prelude's call().
type bridgeReply struct {
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value,omitempty"`
	Error string          `json:"error,omitempty"`
}

// hostBridge implements env.jsFunctionProxy: the guest's only path to the host.
type hostBridge struct{}

// instantiate registers the env module on rt.
func (b hostBridge) instantiate(ctx context.Context, rt wazero.Runtime) error {
	params := []api.ValueType{api.ValueTypeI32, api.ValueTypeI64, api.ValueTypeI32, api.ValueTypeI32}
	_, err := rt.NewHostModuleBuilder(string(moduleEnv)).
		NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(b.handle), params, []api.ValueType{api.ValueTypeI64}).
		Export(string(importProxy)).
		Instantiate(ctx)
	return err
}

// handle answers one proxied call with a JSON envelope string; ABI faults trap the guest.
func (b hostBridge) handle(ctx context.Context, mod api.Module, stack []uint64) {
	state := callStateFrom(ctx)
	if state == nil {
		panic(errInstanceClosed)
	}
	jsctx := api.DecodeU32(stack[0])
	reply := b.dispatch(ctx, state, jsctx, api.DecodeU32(stack[2]), api.DecodeU32(stack[3]))
	raw, err := json.Marshal(reply)
	if err != nil {
		panic(err)
	}
	v, err := state.abi.newString(ctx, jsctx, raw)
	if err != nil {
		panic(err)
	}
	stack[0] = v
}

// dispatch reads op, name and payload, then routes to the input or helper handler.
func (b hostBridge) dispatch(ctx context.Context, s *callState, jsctx, argc, argv uint32) bridgeReply {
	if argc < uint32(proxyArgCount) {
		return bridgeReply{Error: errBadBridgeCall.Error()}
	}
	args := proxyArgs{abi: s.abi, jsctx: jsctx, argv: argv}
	op, err := args.text(ctx, proxyArgOp, int64(bridgeNameLimit))
	if err != nil {
		return bridgeReply{Error: err.Error()}
	}
	if guestOp(op) == opInput {
		return bridgeReply{OK: true, Value: s.input}
	}
	name, err := args.text(ctx, proxyArgName, int64(bridgeNameLimit))
	if err != nil {
		return bridgeReply{Error: err.Error()}
	}
	payload, err := args.text(ctx, proxyArgPayload, s.argLimit)
	if err != nil {
		return bridgeReply{Error: err.Error()}
	}
	call := &host.Call{Ctx: ctx, Now: s.clock.Now(), Logs: s.logs}
	out, err := s.registry.Dispatch(host.FnName(name), []byte(payload), call)
	if err != nil {
		return bridgeReply{Error: err.Error()}
	}
	return bridgeReply{OK: true, Value: out}
}

// proxyArgs reads JS values from the argv array of a proxied call.
type proxyArgs struct {
	abi   *guestABI
	jsctx uint32
	argv  uint32
}

// text converts argument i to a string of at most limit bytes.
func (p proxyArgs) text(ctx context.Context, i proxyArg, limit int64) (string, error) {
	raw, err := p.abi.view(p.argv+uint32(i)*uint32(jsValueBytes), uint32(jsValueBytes))
	if err != nil {
		return "", err
	}
	b, err := p.abi.toString(ctx, p.jsctx, binary.LittleEndian.Uint64(raw), limit)
	return string(b), err
}
