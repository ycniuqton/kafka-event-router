package preview

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/eventrouter/router/internal/platform/controlplane"
	"github.com/eventrouter/router/pkg/kafka"
)

// ConnectionSpecSource resolves a saved connection into a client spec with its secrets.
type ConnectionSpecSource interface {
	Spec(ctx context.Context, connectionID string) (kafka.ConnectionSpec, error)
}

// ClientOpener opens cluster clients (implemented by kafka.ClientFactoryRegistry).
type ClientOpener interface {
	NewClient(ctx context.Context, spec kafka.ConnectionSpec, purpose kafka.ClientPurpose) (kafka.Client, error)
}

// FranzSamplingFactory opens group-less sampler-purpose clients through connections' client factory.
type FranzSamplingFactory struct {
	Specs   ConnectionSpecSource
	Clients ClientOpener
}

func (FranzSamplingFactory) Kind() SamplerKind { return SamplerFranz }

// Open resolves the connection and opens a franz sampling client (no group id, no commits).
func (f FranzSamplingFactory) Open(ctx context.Context, connectionID string) (SamplingClient, error) {
	spec, err := f.Specs.Spec(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	client, err := f.Clients.NewClient(ctx, spec, kafka.PurposeSampler)
	if err != nil {
		return nil, ErrSourceUnavailable.Wrap(err)
	}
	return kafka.NewFranzSamplingClient(client), nil
}

// InternalPath names the control-plane paths preview reads.
type InternalPath string

const PathConnectionSpec InternalPath = "/internal/v1/connections/%s/spec"

// For returns the path of one connection.
func (p InternalPath) For(id string) string { return fmt.Sprintf(string(p), url.PathEscape(id)) }

// JSONFetcher is the control-plane read port (implemented by controlplane.Client).
type JSONFetcher interface {
	GetJSON(ctx context.Context, path string, out any) error
}

// ControlPlaneSpecs reads connection specs from the API internal listener (audited there as secret reads).
type ControlPlaneSpecs struct {
	Fetcher JSONFetcher
}

// Spec implements ConnectionSpecSource; an unknown connection is a 404 at /connectionId.
func (s ControlPlaneSpecs) Spec(ctx context.Context, connectionID string) (kafka.ConnectionSpec, error) {
	var spec kafka.ConnectionSpec
	err := s.Fetcher.GetJSON(ctx, PathConnectionSpec.For(connectionID), &spec)
	var status *controlplane.StatusError
	if errors.As(err, &status) && status.Status == http.StatusNotFound {
		return kafka.ConnectionSpec{}, ErrConnectionGone.At(string(FieldConnectionID))
	}
	if err != nil {
		return kafka.ConnectionSpec{}, ErrSourceUnavailable.Wrap(err)
	}
	return spec, nil
}

// UnreachableSpecs answers every lookup with the error that kept the control-plane client from building.
type UnreachableSpecs struct {
	Err error
}

// Spec implements ConnectionSpecSource.
func (s UnreachableSpecs) Spec(context.Context, string) (kafka.ConnectionSpec, error) {
	return kafka.ConnectionSpec{}, ErrSourceUnavailable.Wrap(s.Err)
}
