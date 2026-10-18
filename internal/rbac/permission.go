package rbac

import (
	"sort"

	"github.com/eventrouter/router/internal/auth"
)

// Permission is "<object>.<verb>"; every permission has exactly one entry in the PermissionCatalog.
type Permission string

const (
	PermRouteRead             Permission = "route.read"
	PermConnectionRead        Permission = "connection.read"
	PermTransformRead         Permission = "transform.read"
	PermMetricsRead           Permission = "metrics.read"
	PermVersionRead           Permission = "version.read"
	PermUserRead              Permission = "user.read"
	PermTeamRead              Permission = "team.read"
	PermAlertRead             Permission = "alert.read"
	PermSampleRead            Permission = "sample.read"
	PermDLQRead               Permission = "dlq.read"
	PermLiveTailRead          Permission = "livetail.read"
	PermRouteWrite            Permission = "route.write"
	PermRouteDeployNonProd    Permission = "route.deploy_nonprod"
	PermRouteDeployProd       Permission = "route.deploy_prod"
	PermDeploymentApprove     Permission = "deployment.approve"
	PermTransformWrite        Permission = "transform.write"
	PermLibraryWrite          Permission = "library.write"
	PermTestCaseWrite         Permission = "testcase.write"
	PermSampleReadRaw         Permission = "sample.read_raw"
	PermDLQAct                Permission = "dlq.act"
	PermAlertWrite            Permission = "alert.write"
	PermConnectionWrite       Permission = "connection.write"
	PermConnectionSecretWrite Permission = "connection.secret_write"
	PermTopicCreate           Permission = "topic.create"
	PermTeamMembersManage     Permission = "team.members_manage"
	PermUserManage            Permission = "user.manage"
	PermTeamManage            Permission = "team.manage"
	PermGrantManage           Permission = "grant.manage"
	PermSettingsManage        Permission = "settings.manage"
	PermAuditRead             Permission = "audit.read"
	PermAuditExport           Permission = "audit.export"
)

// PermissionMeta describes a permission: its scope, its coarse access level, and whether any team
// member may use it on other teams' objects (decision: everyone may read other teams' routes/connections).
type PermissionMeta struct {
	Scope     Scope
	Access    auth.Access
	CrossTeam bool
}

// PermissionCatalog is the single list of known permissions; unknown permissions are denied.
type PermissionCatalog map[Permission]PermissionMeta

// DefaultPermissionCatalog returns the built-in catalog (design §2.1).
func DefaultPermissionCatalog() PermissionCatalog {
	c := PermissionCatalog{}
	c.add(PermissionMeta{Scope: ScopeTeam, Access: auth.AccessRead, CrossTeam: true}, PermRouteRead, PermConnectionRead,
		PermTransformRead, PermMetricsRead, PermVersionRead, PermUserRead, PermTeamRead, PermAlertRead)
	c.add(PermissionMeta{Scope: ScopeTeam, Access: auth.AccessRead}, PermSampleRead, PermDLQRead, PermLiveTailRead, PermSampleReadRaw)
	c.add(PermissionMeta{Scope: ScopeTeam, Access: auth.AccessWrite}, PermRouteWrite, PermRouteDeployNonProd, PermRouteDeployProd,
		PermDeploymentApprove, PermTransformWrite, PermLibraryWrite, PermTestCaseWrite, PermDLQAct, PermAlertWrite)
	c.add(PermissionMeta{Scope: ScopeTeam, Access: auth.AccessAdmin}, PermConnectionWrite, PermConnectionSecretWrite,
		PermTopicCreate, PermTeamMembersManage)
	c.add(PermissionMeta{Scope: ScopeWorkspace, Access: auth.AccessAdmin}, PermUserManage, PermTeamManage, PermGrantManage,
		PermSettingsManage, PermAuditRead, PermAuditExport)
	return c
}

func (c PermissionCatalog) add(meta PermissionMeta, perms ...Permission) {
	for _, p := range perms {
		c[p] = meta
	}
}

// InScope returns every permission of the scope.
func (c PermissionCatalog) InScope(s Scope) PermissionSet {
	set := NewPermissionSet()
	for p, meta := range c {
		if meta.Scope == s {
			set.Add(p)
		}
	}
	return set
}

// builtinCatalog backs Permission.Auth so features can declare RouteSpec permissions from constants.
var builtinCatalog = DefaultPermissionCatalog()

// Auth converts the permission into the platform's RouteSpec permission (key + coarse access).
func (p Permission) Auth() auth.Permission {
	return auth.Permission{Key: string(p), Access: builtinCatalog[p].Access}
}

// PermissionSet is an unordered set of permissions.
type PermissionSet map[Permission]struct{}

// NewPermissionSet builds a set from perms.
func NewPermissionSet(perms ...Permission) PermissionSet {
	s := PermissionSet{}
	for _, p := range perms {
		s.Add(p)
	}
	return s
}

// Add inserts p.
func (s PermissionSet) Add(p Permission) {
	s[p] = struct{}{}
}

// Has reports whether p is in the set.
func (s PermissionSet) Has(p Permission) bool {
	_, ok := s[p]
	return ok
}

// Union returns a new set with the members of s and o.
func (s PermissionSet) Union(o PermissionSet) PermissionSet {
	out := NewPermissionSet()
	for p := range s {
		out.Add(p)
	}
	for p := range o {
		out.Add(p)
	}
	return out
}

// Sorted returns the members in lexical order (stable JSON, golden files).
func (s PermissionSet) Sorted() []Permission {
	out := make([]Permission, 0, len(s))
	for p := range s {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
