// Package specclient resolves lookup source specs from the API internal listener for workers and previews.
package specclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/eventrouter/router/internal/platform/controlplane"
	"github.com/eventrouter/router/pkg/lookup"
)

// InternalPath names the spec endpoint.
type InternalPath string

const PathSourceSpec InternalPath = "/internal/v1/lookup-sources/%s/spec"

// For returns the path of one source.
func (p InternalPath) For(id string) string { return fmt.Sprintf(string(p), url.PathEscape(id)) }

// JSONFetcher is the control-plane read port (implemented by controlplane.Client).
type JSONFetcher interface {
	GetJSON(ctx context.Context, path string, out any) error
}

// RemoteSpecs implements lookup.SpecProvider over the internal listener (workload auth, audited as secret reads).
type RemoteSpecs struct {
	Fetcher JSONFetcher
}

// Spec implements lookup.SpecProvider; a 404 is lookup.ErrSourceNotFound.
func (s RemoteSpecs) Spec(ctx context.Context, id string) (lookup.SourceSpec, error) {
	var spec lookup.SourceSpec
	err := s.Fetcher.GetJSON(ctx, PathSourceSpec.For(id), &spec)
	var status *controlplane.StatusError
	if errors.As(err, &status) && status.Status == http.StatusNotFound {
		return lookup.SourceSpec{}, fmt.Errorf("%w: %s", lookup.ErrSourceNotFound, id)
	}
	return spec, err
}

// Unreachable answers every spec request with the error that kept the control-plane client from building.
type Unreachable struct {
	Err error
}

// Spec implements lookup.SpecProvider.
func (u Unreachable) Spec(context.Context, string) (lookup.SourceSpec, error) {
	return lookup.SourceSpec{}, u.Err
}
