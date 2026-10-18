package workload

import (
	"crypto/subtle"
	"errors"
	"net/http"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
)

// StaticWorkloadName is the principal name of workloads authenticated by the shared local token.
type StaticWorkloadName string

const LocalWorkload StaticWorkloadName = "local-workload"

// StaticTokenAuthenticator checks a shared bearer token (local and staging environments only).
type StaticTokenAuthenticator struct {
	token []byte
}

// NewStaticTokenAuthenticator requires a non-empty token.
func NewStaticTokenAuthenticator(token string) (*StaticTokenAuthenticator, error) {
	if token == "" {
		return nil, errors.New("workload: ROUTER_WORKLOAD_TOKEN is required for static_token")
	}
	return &StaticTokenAuthenticator{token: []byte(token)}, nil
}

// Kind implements Authenticator.
func (a *StaticTokenAuthenticator) Kind() Kind {
	return config.WorkloadStaticToken
}

// Authenticate compares the bearer token in constant time.
func (a *StaticTokenAuthenticator) Authenticate(r *http.Request) (auth.Principal, error) {
	got := BearerToken(r)
	if got == "" || subtle.ConstantTimeCompare([]byte(got), a.token) != 1 {
		return auth.Anonymous, ErrUnauthenticated
	}
	return auth.Principal{Kind: auth.PrincipalWorkload, WorkloadName: string(LocalWorkload)}, nil
}
