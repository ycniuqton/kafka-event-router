// Package apitokentest provides an in-memory apitoken.Repository for API tests.
package apitokentest

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth/apitoken"
	"github.com/eventrouter/router/internal/store"
)

// MemoryRepository keeps tokens in a map; it mirrors the Postgres repository's semantics.
type MemoryRepository struct {
	mu      sync.Mutex
	tokens  map[uuid.UUID]apitoken.Token
	Touched map[uuid.UUID]time.Time
}

// NewMemoryRepository builds an empty repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{tokens: map[uuid.UUID]apitoken.Token{}, Touched: map[uuid.UUID]time.Time{}}
}

// Insert implements apitoken.Repository; a duplicate active name is a store conflict on /name.
func (r *MemoryRepository) Insert(_ context.Context, t apitoken.Token) (apitoken.Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, other := range r.tokens {
		if other.UserID == t.UserID && other.Name == t.Name && other.RevokedAt == nil {
			return apitoken.Token{}, &store.Error{Kind: store.ErrConflict.Kind, Detail: store.ErrConflict.Detail, Path: string(apitoken.FieldName)}
		}
	}
	t.ID, t.CreatedAt = uuid.New(), time.Now()
	r.tokens[t.ID] = t
	return t, nil
}

// ListActive implements apitoken.Repository.
func (r *MemoryRepository) ListActive(_ context.Context, userID uuid.UUID) ([]apitoken.Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []apitoken.Token{}
	for _, t := range r.tokens {
		if t.UserID == userID && t.RevokedAt == nil {
			out = append(out, t)
		}
	}
	return out, nil
}

// CountActive implements apitoken.Repository.
func (r *MemoryRepository) CountActive(ctx context.Context, userID uuid.UUID) (int, error) {
	l, err := r.ListActive(ctx, userID)
	return len(l), err
}

// FindByHash implements apitoken.Repository.
func (r *MemoryRepository) FindByHash(_ context.Context, hash []byte) (apitoken.Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tokens {
		if bytes.Equal(t.Hash, hash) {
			return t, nil
		}
	}
	return apitoken.Token{}, store.ErrNotFound
}

// Revoke implements apitoken.Repository.
func (r *MemoryRepository) Revoke(_ context.Context, userID, id uuid.UUID, at time.Time) (apitoken.Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tokens[id]
	if !ok || t.UserID != userID || t.RevokedAt != nil {
		return apitoken.Token{}, store.ErrNotFound
	}
	t.RevokedAt = &at
	r.tokens[id] = t
	return t, nil
}

// TouchLastUsed implements apitoken.Repository.
func (r *MemoryRepository) TouchLastUsed(_ context.Context, used map[uuid.UUID]time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, at := range used {
		r.Touched[id] = at
	}
	return nil
}
