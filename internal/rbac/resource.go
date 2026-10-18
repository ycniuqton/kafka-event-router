package rbac

import (
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/auth"
)

// ResourceType names the kind of object a permission is checked against.
type ResourceType string

const (
	ResourceRoute       ResourceType = "route"
	ResourceConnection  ResourceType = "connection"
	ResourceTransform   ResourceType = "transform"
	ResourceLookupTable ResourceType = "lookup_table"
	ResourceDeployment  ResourceType = "deployment"
	ResourceTeam        ResourceType = "team"
	ResourceUser        ResourceType = "user"
	ResourceRoleBinding ResourceType = "role_binding"
	ResourceAudit       ResourceType = "audit"
	ResourceSettings    ResourceType = "settings"
)

// Resource is the authorization target; a zero TeamID means a collection or workspace-scoped object.
type Resource struct {
	Type      ResourceType
	ID        string
	TeamID    TeamID
	EnvTag    EnvTag
	Sensitive bool
}

// HasTeam reports whether the resource belongs to a known team.
func (r Resource) HasTeam() bool {
	return r.TeamID != uuid.Nil
}

// ResourceMapper converts platform-core object references into resources.
type ResourceMapper struct{}

// FromObject maps an ObjectRef; an unparsable team id becomes "no team".
func (ResourceMapper) FromObject(obj auth.ObjectRef) Resource {
	team, err := uuid.Parse(obj.TeamID)
	if err != nil {
		team = uuid.Nil
	}
	return Resource{Type: ResourceType(obj.Type), ID: obj.ID, TeamID: team}
}
