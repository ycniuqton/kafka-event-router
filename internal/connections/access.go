package connections

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/kafka"
	"github.com/eventrouter/router/pkg/schemaregistry"
)

// ConnectionAccess is the PORT to access-control's grant check (implemented by rbac.Manager, AC-104).
type ConnectionAccess interface {
	CanUseConnection(ctx context.Context, team uuid.UUID, connID uuid.UUID) (bool, error)
}

// OwnerOnlyAccess is the fallback policy without access-control: only the owner team may use a connection.
type OwnerOnlyAccess struct{}

// CanUseConnection implements ConnectionAccess; the owner shortcut in Service.CanUse already answered.
func (OwnerOnlyAccess) CanUseConnection(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}

// SecretFieldsOfMode lists which secret fields each security mode uses.
var SecretFieldsOfMode = map[kafka.SecurityMode][]kafka.SecretField{
	kafka.SecuritySASLPlain:       {kafka.SecretPassword},
	kafka.SecuritySASLScram256:    {kafka.SecretPassword},
	kafka.SecuritySASLScram512:    {kafka.SecretPassword},
	kafka.SecurityMTLS:            {kafka.SecretClientKeyPEM},
	kafka.SecuritySASLOAuthBearer: {kafka.SecretClientSecret},
}

// SecretUsage decides which secret fields a connection still needs (security mode + registry auth).
type SecretUsage struct {
	Registry *schemaregistry.AuthRegistry
}

// Fields returns the secret fields spec uses.
func (u SecretUsage) Fields(spec kafka.ConnectionSpec) []kafka.SecretField {
	fields := append([]kafka.SecretField{}, SecretFieldsOfMode[spec.SecurityMode]...)
	if u.RegistryNeedsSecret(spec) {
		fields = append(fields, kafka.SecretRegistry)
	}
	return fields
}

// Keep drops the secrets spec no longer uses from bundle.
func (u SecretUsage) Keep(bundle kafka.SecretBundle, spec kafka.ConnectionSpec) kafka.SecretBundle {
	return bundle.Only(u.Fields(spec))
}

// RegistryNeedsSecret reports whether the spec's registry auth mode uses the registry secret.
func (u SecretUsage) RegistryNeedsSecret(spec kafka.ConnectionSpec) bool {
	return u.registry().UsesSecret(spec)
}

func (u SecretUsage) registry() *schemaregistry.AuthRegistry {
	if u.Registry == nil {
		return schemaregistry.DefaultAuthRegistry()
	}
	return u.Registry
}

// CanUse reports whether routes of team may use the connection: the owner always, others through grants.
func (s *Service) CanUse(ctx context.Context, team, id uuid.UUID) (bool, error) {
	c, err := s.d.Repo.Get(ctx, id)
	if err != nil {
		return false, err
	}
	if c.TeamID == team {
		return true, nil
	}
	return s.access().CanUseConnection(ctx, team, id)
}

// TeamOf returns the owner team of a connection (rbac.TeamOwner for team-scoped permission checks).
func (s *Service) TeamOf(ctx context.Context, id string) (string, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return "", store.ErrNotFound
	}
	c, err := s.d.Repo.Get(ctx, parsed)
	if err != nil {
		return "", err
	}
	return c.TeamID.String(), nil
}

func (s *Service) access() ConnectionAccess {
	if s.d.Access == nil {
		return OwnerOnlyAccess{}
	}
	return s.d.Access
}
