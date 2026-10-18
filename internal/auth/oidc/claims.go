package oidc

import (
	"fmt"
	"strings"

	"github.com/eventrouter/router/internal/auth"
)

// ClaimSyntax names the separators of claim paths and mapping entries.
type ClaimSyntax string

const (
	ClaimPathSeparator  ClaimSyntax = "."
	MappingGroupSep     ClaimSyntax = "="
	MappingTeamRoleSep  ClaimSyntax = ":"
	ClaimValueSeparator ClaimSyntax = " "
)

// ClaimPath reads group values from id_token claims at a dot path ("groups", "realm_access.roles");
// arrays of strings, a single string (space separated) and missing claims are accepted.
type ClaimPath string

// Values returns the group names found at the path (nil when absent or of another type).
func (p ClaimPath) Values(claims map[string]any) []string {
	if p == "" {
		return nil
	}
	var node any = claims
	for _, part := range strings.Split(string(p), string(ClaimPathSeparator)) {
		obj, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		node = obj[part]
	}
	return ClaimValue{}.Strings(node)
}

// ClaimValue converts a raw claim value to strings.
type ClaimValue struct{}

// Strings flattens a string or string array claim.
func (ClaimValue) Strings(v any) []string {
	switch t := v.(type) {
	case string:
		return strings.Fields(t)
	case []any:
		var out []string
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// GroupGrant is the team role one IdP group grants.
type GroupGrant struct {
	Team string
	Role auth.Role
}

// RoleMapping is the configured "group=team:role" table (ROUTER_OIDC_ROLE_MAPPINGS).
type RoleMapping struct {
	byGroup map[string][]GroupGrant
}

// ParseRoleMapping validates entries; roles must be known team roles.
func ParseRoleMapping(entries []string, valid func(auth.Role) bool) (*RoleMapping, error) {
	m := &RoleMapping{byGroup: map[string][]GroupGrant{}}
	for _, entry := range entries {
		group, grant, err := m.parse(entry, valid)
		if err != nil {
			return nil, err
		}
		m.byGroup[group] = append(m.byGroup[group], grant)
	}
	return m, nil
}

func (m *RoleMapping) parse(entry string, valid func(auth.Role) bool) (string, GroupGrant, error) {
	group, target, ok := strings.Cut(entry, string(MappingGroupSep))
	team, role, ok2 := strings.Cut(target, string(MappingTeamRoleSep))
	group, team = strings.TrimSpace(group), strings.TrimSpace(team)
	r := auth.Role(strings.ToLower(strings.TrimSpace(role)))
	if !ok || !ok2 || group == "" || team == "" || !valid(r) {
		return "", GroupGrant{}, fmt.Errorf("oidc: role mapping %q must be group=team:role with a known role", entry)
	}
	return group, GroupGrant{Team: team, Role: r}, nil
}

// Empty reports whether no mapping is configured.
func (m *RoleMapping) Empty() bool {
	return len(m.byGroup) == 0
}

// Grants returns every grant of the given groups, in configuration order per group.
func (m *RoleMapping) Grants(groups []string) []GroupGrant {
	var out []GroupGrant
	for _, g := range groups {
		out = append(out, m.byGroup[g]...)
	}
	return out
}
