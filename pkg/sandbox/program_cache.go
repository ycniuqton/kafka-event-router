package sandbox

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// ProgramCacheKind names a program cache implementation selected by configuration.
type ProgramCacheKind string

const (
	ProgramCacheRoute ProgramCacheKind = "route"
	ProgramCacheOff   ProgramCacheKind = "off"
)

// ProgramKey identifies compiled code within one route: content key plus effective limits.
// Versions of the same route (and library transforms used many times) share an entry; routes never do.
type ProgramKey struct {
	RouteID string
	Module  ModuleKey
	Limits  Limits
}

// ProgramKeyOf builds the cache key of src under lim for scope.
func ProgramKeyOf(scope RouteScope, src Source, lim Limits) ProgramKey {
	return ProgramKey{RouteID: scope.RouteID, Module: ModuleKeyOf(src), Limits: lim}
}

// ProgramCompiler compiles one source (implemented by every Runtime).
type ProgramCompiler interface {
	Compile(ctx context.Context, src Source, lim Limits) (Program, error)
}

// ProgramCacheStats reports cache effectiveness: Compiles counts real engine compiles.
type ProgramCacheStats struct {
	Entries  int
	Hits     uint64
	Compiles uint64
}

// ProgramCache is the base of the compiled-program cache domain; every Acquire pairs with a Release.
type ProgramCache interface {
	Acquire(ctx context.Context, key ProgramKey, src Source) (Program, error)
	Release(key ProgramKey)
	Stats() ProgramCacheStats
}

// ProgramCacheFactory builds a cache over the engine compiler.
type ProgramCacheFactory func(compiler ProgramCompiler) ProgramCache

// ProgramCacheRegistry selects the configured cache without switching on concrete types.
type ProgramCacheRegistry struct {
	factories map[ProgramCacheKind]ProgramCacheFactory
}

// NewProgramCacheRegistry registers the built-in caches.
func NewProgramCacheRegistry() ProgramCacheRegistry {
	return ProgramCacheRegistry{factories: map[ProgramCacheKind]ProgramCacheFactory{
		ProgramCacheRoute: func(c ProgramCompiler) ProgramCache { return NewRouteProgramCache(c) },
		ProgramCacheOff:   func(c ProgramCompiler) ProgramCache { return &UncachedPrograms{compiler: c} },
	}}
}

// Build returns the cache of kind; an empty kind means the route cache.
func (r ProgramCacheRegistry) Build(kind ProgramCacheKind, compiler ProgramCompiler) (ProgramCache, error) {
	if kind == "" {
		kind = ProgramCacheRoute
	}
	f, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf(string(MsgUnknownProgramCache), kind)
	}
	return f(compiler), nil
}

// RouteProgramCache compiles each ProgramKey once and keeps it while any module of the route uses it,
// so a hot reload to a version with unchanged JS and repeated library code skip the compile.
type RouteProgramCache struct {
	compiler ProgramCompiler
	mu       sync.Mutex
	entries  map[ProgramKey]*programEntry
	hits     atomic.Uint64
	compiles atomic.Uint64
}

// programEntry is one compiled program; ready closes when the compile finished.
type programEntry struct {
	ready   chan struct{}
	program Program
	err     error
	refs    int
}

// NewRouteProgramCache returns an empty cache.
func NewRouteProgramCache(compiler ProgramCompiler) *RouteProgramCache {
	return &RouteProgramCache{compiler: compiler, entries: map[ProgramKey]*programEntry{}}
}

// Acquire returns the cached program of key or compiles src once; concurrent callers wait.
func (c *RouteProgramCache) Acquire(ctx context.Context, key ProgramKey, src Source) (Program, error) {
	c.mu.Lock()
	entry, found := c.entries[key]
	if !found {
		entry = &programEntry{ready: make(chan struct{})}
		c.entries[key] = entry
	}
	entry.refs++
	c.mu.Unlock()
	if found {
		return c.await(ctx, key, entry)
	}
	c.compiles.Add(1)
	entry.program, entry.err = c.compiler.Compile(ctx, src, key.Limits)
	close(entry.ready)
	if entry.err != nil {
		c.Release(key)
	}
	return entry.program, entry.err
}

// await waits for a compile started by another caller.
func (c *RouteProgramCache) await(ctx context.Context, key ProgramKey, entry *programEntry) (Program, error) {
	select {
	case <-entry.ready:
	case <-ctx.Done():
		c.Release(key)
		return nil, ctx.Err()
	}
	if entry.err != nil {
		c.Release(key)
		return nil, entry.err
	}
	c.hits.Add(1)
	return entry.program, nil
}

// Release drops one reference; the last one evicts the entry.
func (c *RouteProgramCache) Release(key ProgramKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return
	}
	entry.refs--
	if entry.refs <= 0 {
		delete(c.entries, key)
	}
}

// Stats reports entries, hits and compiles.
func (c *RouteProgramCache) Stats() ProgramCacheStats {
	c.mu.Lock()
	n := len(c.entries)
	c.mu.Unlock()
	return ProgramCacheStats{Entries: n, Hits: c.hits.Load(), Compiles: c.compiles.Load()}
}

// UncachedPrograms compiles on every Acquire (benchmarks, diagnosis).
type UncachedPrograms struct {
	compiler ProgramCompiler
	compiles atomic.Uint64
}

func (u *UncachedPrograms) Acquire(ctx context.Context, key ProgramKey, src Source) (Program, error) {
	u.compiles.Add(1)
	return u.compiler.Compile(ctx, src, key.Limits)
}

func (u *UncachedPrograms) Release(ProgramKey) {}

func (u *UncachedPrograms) Stats() ProgramCacheStats {
	return ProgramCacheStats{Compiles: u.compiles.Load()}
}
