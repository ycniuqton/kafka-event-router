// Package codectest provides an in-memory schema registry for codec, engine and
// consumer tests (preview, DLQ) without a network registry.
package codectest

import (
	"context"
	"fmt"
	"sync"

	"github.com/eventrouter/router/pkg/engine/codec"
)

// FirstSchemaID is the id of the first registered schema.
const FirstSchemaID int32 = 1

// MemoryRegistry implements codec.SchemaRegistry and codec.RegistryProvider in memory.
type MemoryRegistry struct {
	mu       sync.Mutex
	byID     map[int32]codec.RegisteredSchema
	subjects map[string][]codec.RegisteredSchema
	nextID   int32
	Down     bool
}

// NewMemoryRegistry returns an empty registry.
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{byID: map[int32]codec.RegisteredSchema{}, subjects: map[string][]codec.RegisteredSchema{}, nextID: FirstSchemaID}
}

// Register adds a new version of subject and returns its id.
func (r *MemoryRegistry) Register(subject string, typ codec.SchemaType, text string, refs ...codec.SchemaReference) int32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := codec.RegisteredSchema{ID: r.nextID, Subject: subject, Version: len(r.subjects[subject]) + 1, Type: typ, Text: text, References: refs}
	r.nextID++
	r.byID[s.ID] = s
	r.subjects[subject] = append(r.subjects[subject], s)
	return s.ID
}

// Registry serves every connection from this registry.
func (r *MemoryRegistry) Registry(context.Context, string) (codec.SchemaRegistry, error) {
	return r, nil
}

func (r *MemoryRegistry) SchemaByID(_ context.Context, id int32) (codec.RegisteredSchema, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Down {
		return codec.RegisteredSchema{}, fmt.Errorf("%w: registry down", codec.ErrTransient)
	}
	s, ok := r.byID[id]
	if !ok {
		return codec.RegisteredSchema{}, fmt.Errorf("%w: id %d", codec.ErrSchemaNotFound, id)
	}
	return s, nil
}

func (r *MemoryRegistry) Latest(ctx context.Context, subject string) (codec.RegisteredSchema, error) {
	return r.Version(ctx, subject, 0)
}

// Version returns version v of subject (0 = latest).
func (r *MemoryRegistry) Version(_ context.Context, subject string, v int) (codec.RegisteredSchema, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Down {
		return codec.RegisteredSchema{}, fmt.Errorf("%w: registry down", codec.ErrTransient)
	}
	versions := r.subjects[subject]
	if v == 0 {
		v = len(versions)
	}
	if v < 1 || v > len(versions) {
		return codec.RegisteredSchema{}, fmt.Errorf("%w: %s v%d", codec.ErrSchemaNotFound, subject, v)
	}
	return versions[v-1], nil
}
