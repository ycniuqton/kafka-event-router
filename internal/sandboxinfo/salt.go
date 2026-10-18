package sandboxinfo

import (
	"context"

	"github.com/eventrouter/router/internal/secrets"
	"github.com/eventrouter/router/pkg/sandbox/host"
)

// SecretsSaltStore adapts internal/secrets.Manager to the sandbox's host.SecretStore port.
type SecretsSaltStore struct {
	Secrets secrets.Manager
}

// Put stores a new salt and returns its secret ref.
func (s SecretsSaltStore) Put(ctx context.Context, plaintext []byte) (string, error) {
	ref, err := s.Secrets.Put(ctx, plaintext)
	return string(ref), err
}

// Get decrypts the salt behind ref.
func (s SecretsSaltStore) Get(ctx context.Context, ref string) ([]byte, error) {
	return s.Secrets.Get(ctx, secrets.Ref(ref))
}

// NewTeamSalts returns the API-side salt provider: teams.hash_salt_ref + secrets, generated on first use.
func NewTeamSalts(m secrets.Manager, refs host.TeamSaltRefStore) host.SaltProvider {
	return host.NewStoreSaltProvider(SecretsSaltStore{Secrets: m}, refs)
}
