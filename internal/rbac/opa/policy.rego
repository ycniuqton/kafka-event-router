# Built-in Event Router authorization policy: mirrors rbac.RoleBindingEngine (design flow 5.1).
# data.router.permissions: {perm: {scope, crossTeam}}; data.router.roles: {role: [perm]}.
package router.authz

import rego.v1

known if input.permission in object.keys(data.router.permissions)

meta := data.router.permissions[input.permission]

has_binding if count(input.subject.bindings) > 0

granted_on_target if {
	input.resource.teamId != ""
	role := input.subject.bindings[input.resource.teamId]
	input.permission in data.router.roles[role]
}

granted_on_target if {
	input.resource.teamId == ""
	some role in input.subject.bindings
	input.permission in data.router.roles[role]
}

decision := {"allowed": false, "reason": "unknown_permission"} if {
	not known
} else := {"allowed": false, "reason": "disabled_user"} if {
	input.subject.disabled
} else := {"allowed": true, "reason": ""} if {
	input.subject.workspaceAdmin
} else := {"allowed": false, "reason": "missing_permission"} if {
	meta.scope == "workspace"
} else := {"allowed": false, "reason": "no_binding"} if {
	not has_binding
} else := {"allowed": true, "reason": ""} if {
	granted_on_target
} else := {"allowed": true, "reason": ""} if {
	meta.crossTeam
} else := {"allowed": false, "reason": "missing_permission"}

permissions := set() if {
	input.subject.disabled
} else := {p | some p, _ in data.router.permissions} if {
	input.subject.workspaceAdmin
} else := {p | some p in data.router.roles[input.subject.bindings[input.team]]} if {
	input.team in object.keys(input.subject.bindings)
} else := set()
