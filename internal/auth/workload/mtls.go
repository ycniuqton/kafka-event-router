package workload

import (
	"crypto/x509"
	"net/http"
	"slices"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
)

// MTLSAuthenticator authenticates workloads by the client certificate the internal listener verified
// against ROUTER_TLS_CLIENT_CA_FILE (CA pinning). The workload name is the first URI SAN (e.g. a SPIFFE id),
// else the DNS SAN, else the subject CN; when an allow-list is configured the name must be on it.
type MTLSAuthenticator struct {
	allowed []string
}

// NewMTLSAuthenticator builds the authenticator; an empty allow-list accepts any certificate the CA signed.
func NewMTLSAuthenticator(allowed []string) *MTLSAuthenticator {
	return &MTLSAuthenticator{allowed: allowed}
}

// Kind implements Authenticator.
func (a *MTLSAuthenticator) Kind() Kind {
	return config.WorkloadMTLS
}

// Authenticate requires a verified client chain (the TLS handshake already checked it against the CA).
func (a *MTLSAuthenticator) Authenticate(r *http.Request) (auth.Principal, error) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.VerifiedChains[0]) == 0 {
		return auth.Anonymous, ErrUnauthenticated
	}
	name := CertificateIdentity{}.Name(r.TLS.VerifiedChains[0][0])
	if name == "" || (len(a.allowed) > 0 && !slices.Contains(a.allowed, name)) {
		return auth.Anonymous, ErrUnauthenticated
	}
	return auth.Principal{Kind: auth.PrincipalWorkload, WorkloadName: name}, nil
}

// CertificateIdentity derives a workload name from a leaf certificate.
type CertificateIdentity struct{}

// Name returns the first URI SAN, else the first DNS SAN, else the subject common name.
func (CertificateIdentity) Name(leaf *x509.Certificate) string {
	if len(leaf.URIs) > 0 {
		return leaf.URIs[0].String()
	}
	if len(leaf.DNSNames) > 0 {
		return leaf.DNSNames[0]
	}
	return leaf.Subject.CommonName
}
