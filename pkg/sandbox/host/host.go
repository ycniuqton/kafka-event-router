package host

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// HelperDoc documents one ctx helper for the runtime-info catalogue and Monaco typings.
type HelperDoc struct {
	Name      FnName `json:"name"`
	Signature string `json:"signature"`
	Doc       string `json:"doc"`
}

// Call carries one helper invocation: JSON arguments plus per-call state.
type Call struct {
	Ctx  context.Context
	Args []json.RawMessage
	Now  time.Time
	Logs *LogBuffer
}

// HostFunction is the base of every ctx helper.
type HostFunction interface {
	Name() FnName
	Doc() HelperDoc
	Invoke(call *Call) (any, error)
}

// Registry dispatches helper calls by name; one registry per route sandbox.
type Registry struct {
	fns   map[FnName]HostFunction
	order []FnName
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{fns: map[FnName]HostFunction{}}
}

// NewStandardRegistry registers every MVP ctx helper bound to services.
func NewStandardRegistry(s HostServices) *Registry {
	r := NewRegistry()
	for _, fn := range StandardFunctions(s) {
		r.Register(fn)
	}
	return r
}

// StandardFunctions lists the MVP helpers in catalogue order.
func StandardFunctions(s HostServices) []HostFunction {
	return []HostFunction{
		MaskFn{style: MaskEmail, name: FnMaskEmail, services: s},
		MaskFn{style: MaskLast4, name: FnMaskLast4, services: s},
		MaskFn{style: MaskFull, name: FnMaskFull, services: s},
		HashFn{services: s},
		NowFn{},
		FormatDateFn{services: s},
		LookupFn{services: s},
		LogFn{},
	}
}

// Register adds or replaces a helper.
func (r *Registry) Register(fn HostFunction) {
	if _, ok := r.fns[fn.Name()]; !ok {
		r.order = append(r.order, fn.Name())
	}
	r.fns[fn.Name()] = fn
}

// Docs returns helper docs in registration order.
func (r *Registry) Docs() []HelperDoc {
	out := make([]HelperDoc, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.fns[n].Doc())
	}
	return out
}

// Dispatch decodes the JSON argument array, runs the helper and encodes its result.
func (r *Registry) Dispatch(name FnName, args []byte, call *Call) (json.RawMessage, error) {
	fn, ok := r.fns[name]
	if !ok {
		return nil, fmt.Errorf(string(MsgUnknownHelper), name)
	}
	if err := json.Unmarshal(args, &call.Args); err != nil {
		return nil, fmt.Errorf(string(MsgBadArguments), name, err)
	}
	out, err := fn.Invoke(call)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}
