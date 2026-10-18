package compilectx

import (
	"context"
	"fmt"
	"sync"

	"github.com/eventrouter/router/pkg/config"
)

// LibraryVersionSeparator joins a library ref and its pinned version (`orders-clean@3`).
const LibraryVersionSeparator = "@"

// LibraryPin is one pinned library entry (transform or lookup table).
type LibraryPin struct {
	Ref     string
	Version int
}

// String renders `<ref>@<version>`, the name used in step ids and lookup table names.
func (p LibraryPin) String() string {
	return fmt.Sprintf("%s%s%d", p.Ref, LibraryVersionSeparator, p.Version)
}

// LibraryCache memoises a LibraryResolver for one compile so a pin referenced by
// several steps or branches is loaded once.
type LibraryCache struct {
	Resolver   config.LibraryResolver
	mu         sync.Mutex
	transforms map[LibraryPin]libraryTransform
	tables     map[LibraryPin]libraryTable
}

type libraryTransform struct {
	steps config.Steps
	err   error
}

type libraryTable struct {
	rows config.LookupRows
	err  error
}

// NewLibraryCache wraps r; a nil r yields a nil cache (library features unavailable).
func NewLibraryCache(r config.LibraryResolver) config.LibraryResolver {
	if r == nil {
		return nil
	}
	return &LibraryCache{Resolver: r, transforms: map[LibraryPin]libraryTransform{}, tables: map[LibraryPin]libraryTable{}}
}

// Transform returns the pinned steps, loading them once.
func (c *LibraryCache) Transform(ctx context.Context, ref string, version int) (config.Steps, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pin := LibraryPin{Ref: ref, Version: version}
	if hit, ok := c.transforms[pin]; ok {
		return hit.steps, hit.err
	}
	steps, err := c.Resolver.Transform(ctx, ref, version)
	c.transforms[pin] = libraryTransform{steps: steps, err: err}
	return steps, err
}

// LookupTable returns the pinned rows, loading them once.
func (c *LibraryCache) LookupTable(ctx context.Context, ref string, version int) (config.LookupRows, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pin := LibraryPin{Ref: ref, Version: version}
	if hit, ok := c.tables[pin]; ok {
		return hit.rows, hit.err
	}
	rows, err := c.Resolver.LookupTable(ctx, ref, version)
	c.tables[pin] = libraryTable{rows: rows, err: err}
	return rows, err
}
