package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/eventrouter/router/internal/platform/controlplane"
	"github.com/eventrouter/router/pkg/config"
)

// JSONFetcher reads a JSON document from the API internal listener (controlplane.Client).
type JSONFetcher interface {
	GetJSON(ctx context.Context, path string, out any) error
}

// ControlPlaneResolver implements config.LibraryResolver for workloads without database access by
// reading pinned versions from the control plane; versions are immutable, so hits are cached forever.
type ControlPlaneResolver struct {
	fetcher JSONFetcher
	mu      sync.Mutex
	cache   map[Pin]json.RawMessage
}

// NewControlPlaneResolver builds the resolver over the workload's control-plane client.
func NewControlPlaneResolver(fetcher JSONFetcher) *ControlPlaneResolver {
	return &ControlPlaneResolver{fetcher: fetcher, cache: map[Pin]json.RawMessage{}}
}

// Transform implements config.LibraryResolver.
func (r *ControlPlaneResolver) Transform(ctx context.Context, ref string, version int) (config.Steps, error) {
	var steps config.Steps
	return steps, r.decode(ctx, Pin{Kind: PinTransform, Name: ref, Version: version}, &steps)
}

// LookupTable implements config.LibraryResolver.
func (r *ControlPlaneResolver) LookupTable(ctx context.Context, ref string, version int) (config.LookupRows, error) {
	var rows config.LookupRows
	return rows, r.decode(ctx, Pin{Kind: PinLookupTable, Name: ref, Version: version}, &rows)
}

// decode loads the pin (cached) into out; a 404 wraps config.ErrLibraryRefMissing, other failures are transient.
func (r *ControlPlaneResolver) decode(ctx context.Context, pin Pin, out any) error {
	raw, err := r.content(ctx, pin)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func (r *ControlPlaneResolver) content(ctx context.Context, pin Pin) (json.RawMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if raw, ok := r.cache[pin]; ok {
		return raw, nil
	}
	var raw json.RawMessage
	err := r.fetcher.GetJSON(ctx, pin.Path(), &raw)
	var status *controlplane.StatusError
	if errors.As(err, &status) && status.Status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s@%d", config.ErrLibraryRefMissing, pin.Name, pin.Version)
	}
	if err != nil {
		return nil, fmt.Errorf("library: %s %s@%d: %w", pin.Kind, pin.Name, pin.Version, err)
	}
	r.cache[pin] = raw
	return raw, nil
}

var _ config.LibraryResolver = (*ControlPlaneResolver)(nil)
