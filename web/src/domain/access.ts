import { RoleKeys, type RoleKey } from './member';

/** Permission keys (mirror of Go `rbac.Permission`; golden: testdata/rbac/matrix.golden.json). */
export const Permission = {
  AlertRead: 'alert.read',
  AlertWrite: 'alert.write',
  AuditExport: 'audit.export',
  AuditRead: 'audit.read',
  ConnectionRead: 'connection.read',
  ConnectionSecretWrite: 'connection.secret_write',
  ConnectionWrite: 'connection.write',
  DeploymentApprove: 'deployment.approve',
  DlqAct: 'dlq.act',
  DlqRead: 'dlq.read',
  GrantManage: 'grant.manage',
  LibraryWrite: 'library.write',
  LiveTailRead: 'livetail.read',
  MetricsRead: 'metrics.read',
  RouteDeployNonProd: 'route.deploy_nonprod',
  RouteDeployProd: 'route.deploy_prod',
  RouteRead: 'route.read',
  RouteWrite: 'route.write',
  SampleRead: 'sample.read',
  SampleReadRaw: 'sample.read_raw',
  SettingsManage: 'settings.manage',
  TeamManage: 'team.manage',
  TeamMembersManage: 'team.members_manage',
  TeamRead: 'team.read',
  TestCaseWrite: 'testcase.write',
  TopicCreate: 'topic.create',
  TransformRead: 'transform.read',
  TransformWrite: 'transform.write',
  UserManage: 'user.manage',
  UserRead: 'user.read',
  VersionRead: 'version.read',
} as const;
export type Permission = (typeof Permission)[keyof typeof Permission];

/** The caller's role and permissions in one team. */
export type TeamAccess = { role: RoleKey; permissions: Permission[] };

/** Effective access from `GET /auth/me` (UI gating only; the server stays the authority). */
export type Access = { workspaceAdmin: boolean; workspace: Permission[]; teams: Record<string, TeamAccess> };

/** How the builder's deploy button behaves for the caller (P2 approvals). */
export const DeployMode = { Deploy: 'deploy', RequestApproval: 'request_approval', Disabled: 'disabled' } as const;
export type DeployMode = (typeof DeployMode)[keyof typeof DeployMode];

/** Role → permission matrix and workspace-admin permissions (same data as the Go `RoleMatrix`). */
export class RoleMatrix {
  private static readonly viewer: readonly Permission[] = [
    Permission.AlertRead, Permission.ConnectionRead, Permission.DlqRead, Permission.LiveTailRead, Permission.MetricsRead,
    Permission.RouteRead, Permission.SampleRead, Permission.TeamRead, Permission.TransformRead, Permission.UserRead,
    Permission.VersionRead,
  ];
  private static readonly editor: readonly Permission[] = [
    ...RoleMatrix.viewer, Permission.AlertWrite, Permission.DlqAct, Permission.LibraryWrite, Permission.RouteDeployNonProd,
    Permission.RouteWrite, Permission.SampleReadRaw, Permission.TestCaseWrite, Permission.TransformWrite,
  ];
  private static readonly approver: readonly Permission[] = [...RoleMatrix.editor, Permission.DeploymentApprove, Permission.RouteDeployProd];
  private static readonly admin: readonly Permission[] = [
    ...RoleMatrix.approver, Permission.ConnectionSecretWrite, Permission.ConnectionWrite, Permission.TeamMembersManage, Permission.TopicCreate,
  ];
  static readonly roles: Readonly<Record<RoleKey, readonly Permission[]>> = {
    viewer: RoleMatrix.viewer,
    editor: RoleMatrix.editor,
    approver: RoleMatrix.approver,
    admin: RoleMatrix.admin,
  };
  static readonly workspaceAdmin: readonly Permission[] = [
    Permission.AuditExport, Permission.AuditRead, Permission.GrantManage, Permission.SettingsManage, Permission.TeamManage, Permission.UserManage,
  ];
  /** Reads any member may do on other teams' objects (decision: everyone may read routes/connections). */
  static readonly crossTeamReads: readonly Permission[] = [
    Permission.AlertRead, Permission.ConnectionRead, Permission.MetricsRead, Permission.RouteRead, Permission.TeamRead,
    Permission.TransformRead, Permission.UserRead, Permission.VersionRead,
  ];

  /** Builds the access summary of a user from bindings (used by the mock backend). */
  static accessFor(workspaceAdmin: boolean, bindings: Readonly<Record<string, RoleKey>>): Access {
    const teams: Record<string, TeamAccess> = {};
    Object.entries(bindings).forEach(([teamId, role]) => {
      teams[teamId] = { role, permissions: [...RoleMatrix.roles[role]].sort() };
    });
    return { workspaceAdmin, workspace: workspaceAdmin ? [...RoleMatrix.workspaceAdmin] : [], teams };
  }
}

/** Client-side mirror of the policy engine: workspace admin → all; team perms by binding; cross-team reads. */
export class AccessPolicy {
  static can(access: Access | null, permission: Permission, teamId?: string | null): boolean {
    if (!access) return false;
    if (access.workspaceAdmin) return true;
    if (RoleMatrix.workspaceAdmin.includes(permission)) return false;
    const teams = Object.entries(access.teams);
    if (teams.length > 0 && RoleMatrix.crossTeamReads.includes(permission)) return true;
    if (teamId) return access.teams[teamId]?.permissions.includes(permission) ?? false;
    return teams.some(([, t]) => t.permissions.includes(permission));
  }

  /** Lowest role that grants a permission, for "Requires Editor role" hints (null: workspace admin only). */
  static requiredRole(permission: Permission): RoleKey | null {
    return RoleKeys.ordered.find((r) => RoleMatrix.roles[r].includes(permission)) ?? null;
  }
}
