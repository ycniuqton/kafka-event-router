package sandbox

import (
	"context"
	"fmt"
	"sync"

	"github.com/eventrouter/router/pkg/sandbox/determinism"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// Source is one piece of user code; StepPath is its JSON Pointer in the route document.
type Source struct {
	StepPath string
	Entry    EntryKind
	Code     string
}

// Runtime is the base of the sandbox engine domain (one per process, selected by EngineKind).
type Runtime interface {
	Kind() EngineKind
	Check(ctx context.Context, src Source) []Diagnostic
	Compile(ctx context.Context, src Source, lim Limits) (Program, error)
	Close(ctx context.Context) error
}

// Program is compiled user code plus its limits; immutable and shared by a pool.
type Program interface {
	Source() Source
	Limits() Limits
	NewInstance(ctx context.Context, registry *host.Registry) (Instance, error)
}

// GuestCall is one engine-level call: the marshalled event plus determinism inputs.
type GuestCall struct {
	Input     []byte
	Predicate bool
	Clock     determinism.FrozenClock
	Seed      uint64
	Logs      *host.LogBuffer
}

// GuestReply is the raw result of a successful call.
type GuestReply struct {
	Output []byte
	Bool   bool
}

// Instance is one isolated guest (own linear memory); used by one call at a time.
type Instance interface {
	Invoke(ctx context.Context, call GuestCall) (GuestReply, error)
	MemoryBytes() int64
	Reusable() bool
	Close(ctx context.Context) error
}

// RuntimeOptions configure an engine at construction.
type RuntimeOptions struct {
	ResetPolicy ResetPolicy
	CacheDir    string
	CheckLimits Limits
	Prelude     determinism.Prelude
}

// RuntimeFactory constructs a Runtime; engines register one per EngineKind.
type RuntimeFactory func(ctx context.Context, opts RuntimeOptions) (Runtime, error)

// EngineRegistry selects the configured engine without switching on concrete types.
type EngineRegistry struct {
	mu        sync.RWMutex
	factories map[EngineKind]RuntimeFactory
}

// NewEngineRegistry returns an empty registry.
func NewEngineRegistry() *EngineRegistry {
	return &EngineRegistry{factories: map[EngineKind]RuntimeFactory{}}
}

// Register adds the factory of kind.
func (r *EngineRegistry) Register(kind EngineKind, f RuntimeFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[kind] = f
}

// Build instantiates the engine named in configuration.
func (r *EngineRegistry) Build(ctx context.Context, kind EngineKind, opts RuntimeOptions) (Runtime, error) {
	r.mu.RLock()
	f, ok := r.factories[kind]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf(string(MsgUnknownEngine), kind)
	}
	return f(ctx, opts)
}
