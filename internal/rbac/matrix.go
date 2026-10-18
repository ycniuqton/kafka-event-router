package rbac

// RoleMatrix maps each team role to its permissions; it is data, never code branches.
type RoleMatrix map[Role]PermissionSet

// DefaultRoleMatrix returns the built-in matrix: each role extends the previous one (Approver = Editor ∪ approve).
func DefaultRoleMatrix() RoleMatrix {
	viewer := NewPermissionSet(PermRouteRead, PermConnectionRead, PermTransformRead, PermMetricsRead, PermVersionRead,
		PermUserRead, PermTeamRead, PermAlertRead, PermSampleRead, PermDLQRead, PermLiveTailRead)
	editor := viewer.Union(NewPermissionSet(PermRouteWrite, PermRouteDeployNonProd, PermTransformWrite, PermLibraryWrite,
		PermTestCaseWrite, PermSampleReadRaw, PermDLQAct, PermAlertWrite))
	approver := editor.Union(NewPermissionSet(PermRouteDeployProd, PermDeploymentApprove))
	admin := approver.Union(NewPermissionSet(PermConnectionWrite, PermConnectionSecretWrite, PermTopicCreate, PermTeamMembersManage))
	return RoleMatrix{RoleViewer: viewer, RoleEditor: editor, RoleApprover: approver, RoleAdmin: admin}
}

// Permissions returns the role's permissions (empty for unknown roles).
func (m RoleMatrix) Permissions(r Role) PermissionSet {
	if set, ok := m[r]; ok {
		return set
	}
	return NewPermissionSet()
}

// LowestRoleWith returns the least privileged role granting p, for "you need the X role" messages.
func (m RoleMatrix) LowestRoleWith(p Permission) (Role, bool) {
	for _, r := range (Roles{}).Ordered() {
		if m.Permissions(r).Has(p) {
			return r, true
		}
	}
	return "", false
}

// MatrixDocument is the serialised matrix (golden file shared with the FE mock).
type MatrixDocument struct {
	Roles          map[Role][]Permission `json:"roles"`
	WorkspaceAdmin []Permission          `json:"workspaceAdmin"`
}

// Document renders the matrix and the workspace-admin permissions of catalog.
func (m RoleMatrix) Document(catalog PermissionCatalog) MatrixDocument {
	doc := MatrixDocument{Roles: map[Role][]Permission{}, WorkspaceAdmin: catalog.InScope(ScopeWorkspace).Sorted()}
	for r, set := range m {
		doc.Roles[r] = set.Sorted()
	}
	return doc
}
