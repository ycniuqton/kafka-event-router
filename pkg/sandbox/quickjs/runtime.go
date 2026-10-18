package quickjs

import (
	"context"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"

	"github.com/eventrouter/router/pkg/sandbox"
)

// engineTiming holds engine-level time budgets.
type engineTiming time.Duration

const instanceLoadTimeout engineTiming = engineTiming(2 * time.Second)

func (t engineTiming) duration() time.Duration { return time.Duration(t) }

// memoryBudget splits an instance memory cap between the JS heap and pre-grown headroom.
type memoryBudget int64

const (
	wasmPageBytes      memoryBudget = 64 * 1024
	heapShareNumerator memoryBudget = 3
	headroomDivisor    memoryBudget = 4
)

// Runtime is the quickjs_wazero engine; one per process, shared by every route.
type Runtime struct {
	guest      GuestModule
	opts       sandbox.RuntimeOptions
	cache      wazero.CompilationCache
	classifier sandbox.ErrorClassifier
	mu         sync.Mutex
	groups     map[uint32]*engineGroup
	compiler   *compilerVM
}

// Register adds the engine to an EngineRegistry under quickjs_wazero.
func Register(reg *sandbox.EngineRegistry) {
	reg.Register(sandbox.EngineQuickJSWazero, func(ctx context.Context, opts sandbox.RuntimeOptions) (sandbox.Runtime, error) {
		return New(ctx, opts)
	})
}

// New verifies the embedded guest and prepares the compilation cache.
func New(_ context.Context, opts sandbox.RuntimeOptions) (*Runtime, error) {
	guest := EmbeddedGuest()
	if err := guest.Verify(); err != nil {
		return nil, err
	}
	cache := wazero.NewCompilationCache()
	if opts.CacheDir != "" {
		dirCache, err := wazero.NewCompilationCacheWithDir(opts.CacheDir)
		if err != nil {
			return nil, err
		}
		cache = dirCache
	}
	r := &Runtime{guest: guest, opts: opts, cache: cache, classifier: sandbox.NewErrorClassifier(),
		groups: map[uint32]*engineGroup{}}
	r.compiler = &compilerVM{rt: r}
	return r, nil
}

func (r *Runtime) Kind() sandbox.EngineKind { return sandbox.EngineQuickJSWazero }

// Close closes every wazero runtime and the compilation cache.
func (r *Runtime) Close(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var first error
	for _, g := range r.groups {
		first = joinNil(first, g.rt.Close(ctx))
	}
	r.groups = map[uint32]*engineGroup{}
	return joinNil(first, r.cache.Close(ctx))
}

// group returns the wazero runtime whose page limit matches memoryBytes.
func (r *Runtime) group(ctx context.Context, memoryBytes int64) (*engineGroup, error) {
	pages := uint32(memoryBytes / int64(wasmPageBytes))
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.groups[pages]; ok {
		return g, nil
	}
	g, err := newEngineGroup(ctx, r.guest.Wasm, r.cache, pages)
	if err != nil {
		return nil, err
	}
	r.groups[pages] = g
	return g, nil
}

// engineGroup is a wazero runtime with one memory cap, the compiled guest and host modules.
type engineGroup struct {
	rt       wazero.Runtime
	compiled wazero.CompiledModule
	mu       sync.Mutex
	boot     *bootTemplate
}

func newEngineGroup(ctx context.Context, wasm []byte, cache wazero.CompilationCache, pages uint32) (*engineGroup, error) {
	cfg := wazero.NewRuntimeConfig().
		WithCompilationCache(cache).
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(pages)
	rt := wazero.NewRuntimeWithConfig(ctx, cfg)
	compiled, err := rt.CompileModule(ctx, wasm)
	if err == nil {
		err = wasiStubs{}.instantiate(ctx, rt, compiled)
	}
	if err == nil {
		err = hostBridge{}.instantiate(ctx, rt)
	}
	if err != nil {
		_ = rt.Close(ctx)
		return nil, err
	}
	return &engineGroup{rt: rt, compiled: compiled}, nil
}

// newModule instantiates an anonymous guest: no args, env, files, stdio or start function.
func (g *engineGroup) newModule(ctx context.Context) (api.Module, error) {
	ctx = experimental.WithMemoryAllocator(ctx, guestMemoryAllocator())
	return g.rt.InstantiateModule(ctx, g.compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions())
}

// budget derives the JS heap limit and the snapshot headroom from a memory cap.
type budget struct {
	heapLimit int64
	headroom  int64
}

func budgetFor(memoryBytes int64) budget {
	return budget{
		heapLimit: memoryBytes * int64(heapShareNumerator) / int64(headroomDivisor),
		headroom:  memoryBytes / int64(headroomDivisor),
	}
}
