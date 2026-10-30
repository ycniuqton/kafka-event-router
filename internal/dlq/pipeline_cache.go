package dlq

import (
	"container/list"
	"context"
	"sync"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine"
	"github.com/eventrouter/router/pkg/engine/codec"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/script"
)

// CompiledVersion is a compiled active version plus its branch destination connections.
type CompiledVersion struct {
	Pipeline     engine.Pipeline
	Destinations map[engine.BranchID]string
	VersionNo    int
}

// ScriptRuntimes supplies the JS runtime for a route (nil runtime: js steps fail to compile).
type ScriptRuntimes interface {
	Runtime(route RouteInfo, version VersionConfig) script.Runtime
}

// NoScriptRuntimes disables js steps.
type NoScriptRuntimes struct{}

// Runtime implements ScriptRuntimes.
func (NoScriptRuntimes) Runtime(RouteInfo, VersionConfig) script.Runtime { return nil }

// PipelineCache is an LRU of compiled versions by version id (design §2.2).
type PipelineCache struct {
	compiler engine.Compiler
	scripts  ScriptRuntimes
	salts    ctxhelpers.SaltProvider
	regs     codec.RegistryProvider
	library  config.LibraryResolver
	sources  ctxhelpers.SourceGateway
	size     int
	mu       sync.Mutex
	order    *list.List
	entries  map[string]*list.Element
}

// cacheEntry is one LRU element.
type cacheEntry struct {
	id      string
	version *CompiledVersion
}

// NewPipelineCache builds a cache of PipelineCacheSize entries.
func NewPipelineCache(compiler engine.Compiler, scripts ScriptRuntimes, salts ctxhelpers.SaltProvider) *PipelineCache {
	return &PipelineCache{compiler: compiler, scripts: scripts, salts: salts, size: int(PipelineCacheSize), order: list.New(), entries: map[string]*list.Element{}}
}

// WithRegistries makes compiled versions decode/encode Avro and Protobuf through connection schema registries.
func (c *PipelineCache) WithRegistries(regs codec.RegistryProvider) *PipelineCache {
	c.regs = regs
	return c
}

// WithLibrary makes compiled versions resolve library steps and named lookup tables.
func (c *PipelineCache) WithLibrary(library config.LibraryResolver) *PipelineCache {
	c.library = library
	return c
}

// WithSources makes compiled versions serve lookup steps in source mode (dry-run and retry, CN-LS-19).
func (c *PipelineCache) WithSources(sources ctxhelpers.SourceGateway) *PipelineCache {
	c.sources = sources
	return c
}

// Get returns the compiled version, compiling on a miss; compile failures are *engine.CompileError.
func (c *PipelineCache) Get(ctx context.Context, route RouteInfo, v VersionConfig) (*CompiledVersion, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[v.ID]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*cacheEntry).version, nil
	}
	compiled, err := c.compile(ctx, route, v)
	if err != nil {
		return nil, err
	}
	c.entries[v.ID] = c.order.PushFront(&cacheEntry{id: v.ID, version: compiled})
	c.evict(ctx)
	return compiled, nil
}

func (c *PipelineCache) compile(ctx context.Context, route RouteInfo, v VersionConfig) (*CompiledVersion, error) {
	doc, err := config.Parse(v.Config)
	if err != nil {
		return nil, &engine.CompileError{Issues: config.Issues{config.NewError(config.IssueSchemaInvalid, config.PointerRoot, "%v", err)}}
	}
	cfg, err := doc.Config()
	if err != nil {
		return nil, &engine.CompileError{Issues: config.Issues{config.NewError(config.IssueSchemaInvalid, config.PointerRoot, "%v", err)}}
	}
	opts := engine.CompileOptions{
		Scripts:          c.scripts.Runtime(route, v),
		Helpers:          ctxhelpers.Set{Hash: ctxhelpers.NewSaltedHasher(c.salts, route.TeamID), Sources: c.sources}.WithDefaults(),
		Meta:             engine.RouteMeta{RouteID: route.ID, Name: route.Name, Version: v.No, TeamID: route.TeamID},
		SchemaRegistries: c.regs,
		Library:          c.library,
	}
	p, err := c.compiler.Compile(ctx, doc, opts)
	if err != nil {
		return nil, err
	}
	dests := map[engine.BranchID]string{}
	for _, b := range cfg.Branches {
		dests[engine.BranchID(b.ID)] = b.Destination.ConnectionID
	}
	return &CompiledVersion{Pipeline: p, Destinations: dests, VersionNo: v.No}, nil
}

// evict closes and drops the least recently used entries above the size.
func (c *PipelineCache) evict(ctx context.Context) {
	for c.order.Len() > c.size {
		el := c.order.Back()
		e := el.Value.(*cacheEntry)
		c.order.Remove(el)
		delete(c.entries, e.id)
		_ = e.version.Pipeline.Close(ctx)
	}
}
