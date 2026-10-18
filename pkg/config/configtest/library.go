// Package configtest provides in-memory fakes of pkg/config ports for tests and tools.
package configtest

import (
	"context"
	"fmt"
	"sync"

	"github.com/eventrouter/router/pkg/config"
)

// LibraryKey addresses one pinned library entry.
type LibraryKey struct {
	Ref     string
	Version int
}

// InMemoryLibrary is a LibraryResolver backed by maps.
type InMemoryLibrary struct {
	mu         sync.RWMutex
	transforms map[LibraryKey]config.Steps
	tables     map[LibraryKey]config.LookupRows
}

// NewInMemoryLibrary returns an empty library.
func NewInMemoryLibrary() *InMemoryLibrary {
	return &InMemoryLibrary{transforms: map[LibraryKey]config.Steps{}, tables: map[LibraryKey]config.LookupRows{}}
}

// PutTransform stores a transform version.
func (l *InMemoryLibrary) PutTransform(ref string, version int, steps config.Steps) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.transforms[LibraryKey{ref, version}] = steps
}

// PutLookupTable stores a lookup table version.
func (l *InMemoryLibrary) PutLookupTable(ref string, version int, rows config.LookupRows) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tables[LibraryKey{ref, version}] = rows
}

// Transform returns the stored steps or ErrLibraryRefMissing.
func (l *InMemoryLibrary) Transform(_ context.Context, ref string, version int) (config.Steps, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	steps, ok := l.transforms[LibraryKey{ref, version}]
	if !ok {
		return nil, fmt.Errorf("%w: %s@%d", config.ErrLibraryRefMissing, ref, version)
	}
	return steps, nil
}

// LookupTable returns the stored rows or ErrLibraryRefMissing.
func (l *InMemoryLibrary) LookupTable(_ context.Context, ref string, version int) (config.LookupRows, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	rows, ok := l.tables[LibraryKey{ref, version}]
	if !ok {
		return nil, fmt.Errorf("%w: %s@%d", config.ErrLibraryRefMissing, ref, version)
	}
	return rows, nil
}
