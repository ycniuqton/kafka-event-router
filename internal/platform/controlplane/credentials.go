package controlplane

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/eventrouter/router/internal/platform/config"
)

// CredentialFactory builds a credential from configuration.
type CredentialFactory func(cp config.ControlPlaneConfig) (Credential, error)

// CredentialRegistry selects the workload credential by the configured kind.
type CredentialRegistry struct {
	factories map[config.WorkloadAuthKind]CredentialFactory
}

// NewCredentialRegistry returns the MVP credentials (static token, projected SA token file).
func NewCredentialRegistry() *CredentialRegistry {
	r := &CredentialRegistry{factories: map[config.WorkloadAuthKind]CredentialFactory{}}
	r.Register(config.WorkloadStaticToken, func(cp config.ControlPlaneConfig) (Credential, error) {
		if cp.WorkloadToken == "" {
			return nil, errors.New("controlplane: ROUTER_WORKLOAD_TOKEN is required for static_token")
		}
		return StaticCredential(cp.WorkloadToken), nil
	})
	r.Register(config.WorkloadK8sToken, func(cp config.ControlPlaneConfig) (Credential, error) {
		if cp.TokenFile == "" {
			return nil, errors.New("controlplane: ROUTER_WORKLOAD_TOKEN_FILE is required for k8s_token")
		}
		return ProjectedTokenFile(cp.TokenFile), nil
	})
	r.Register(config.WorkloadMTLS, func(cp config.ControlPlaneConfig) (Credential, error) {
		if cp.ClientCertFile == "" || cp.ClientKeyFile == "" {
			return nil, errors.New("controlplane: ROUTER_CONTROL_PLANE_CLIENT_CERT_FILE and _KEY_FILE are required for mtls")
		}
		return CertificateCredential{}, nil
	})
	return r
}

// Register adds a credential factory.
func (r *CredentialRegistry) Register(kind config.WorkloadAuthKind, f CredentialFactory) {
	r.factories[kind] = f
}

// Build instantiates the configured credential.
func (r *CredentialRegistry) Build(kind config.WorkloadAuthKind, cp config.ControlPlaneConfig) (Credential, error) {
	f, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf("controlplane: no credential registered for %q", kind)
	}
	return f(cp)
}

// StaticCredential is a shared local token.
type StaticCredential string

// Token implements Credential.
func (s StaticCredential) Token(context.Context) (string, error) {
	return string(s), nil
}

// ProjectedTokenFile re-reads a kubelet-rotated ServiceAccount token on every call.
type ProjectedTokenFile string

// Token implements Credential.
func (p ProjectedTokenFile) Token(context.Context) (string, error) {
	raw, err := os.ReadFile(string(p))
	if err != nil {
		return "", fmt.Errorf("controlplane: read workload token: %w", err)
	}
	return string(bytes.TrimSpace(raw)), nil
}

// CertificateCredential is the mtls credential: identity travels in the client certificate, no bearer token.
type CertificateCredential struct{}

// Token implements Credential (empty: no Authorization header is sent).
func (CertificateCredential) Token(context.Context) (string, error) {
	return "", nil
}
