import { RoleKeys, UserStatus, type Access, type RoleKey, type TeamMember, type WorkspaceSettings } from '../../../domain';

/** `UserTeam` of `GET /users` (paths/access-control.yaml). */
export type UserTeamDto = { teamId: string; teamName: string; role: RoleKey; rowVersion: number };

/** `User` of `GET /users`. */
export type UserDto = {
  id: string;
  email: string;
  displayName: string;
  workspaceAdmin: boolean;
  status: UserStatus;
  teams: UserTeamDto[];
  rowVersion: number;
};

/** `RoleBinding` of `PUT /role-bindings/{userId}/{teamId}`. */
export type RoleBindingDto = { userId: string; teamId: string; teamName: string; role: RoleKey; rowVersion: number };

/** `WorkspaceSettings` of `GET/PATCH /workspace-settings`. */
export type WorkspaceSettingsDto = { aiAssistEnabled: boolean; defaultRequireProdApproval: boolean; rowVersion: number };

/** The `access` member of `GET /auth/me`. */
export type MeAccessDto = { access: Access };

/** REST paths of member administration (relative to `/api/v1`). */
export class AccessEndpoint {
  static readonly me = '/auth/me';
  static readonly users = '/users';
  static readonly invitations = '/users/invitations';
  static readonly settings = '/workspace-settings';

  static user(userId: string): string {
    return `${AccessEndpoint.users}/${encodeURIComponent(userId)}`;
  }

  static binding(userId: string, teamId: string): string {
    return `/role-bindings/${encodeURIComponent(userId)}/${encodeURIComponent(teamId)}`;
  }
}

/** Row versions as strong entity tags for If-Match. */
export class EntityTag {
  static of(version: number | null): string | null {
    return version === null ? null : JSON.stringify(String(version));
  }
}

/** Maps API users onto Settings member rows: one row per team binding, one team-less row otherwise. */
export class MemberMapper {
  static readonly noTeam = '—';

  static rows(users: UserDto[]): TeamMember[] {
    return users.flatMap((u) => (u.teams.length > 0 ? u.teams.map((t) => MemberMapper.row(u, t)) : [MemberMapper.row(u, null)]));
  }

  static row(u: UserDto, t: UserTeamDto | null): TeamMember {
    return {
      name: u.displayName || u.email,
      role: RoleKeys.toRole(t?.role ?? RoleKeys.ordered[0]),
      team: t?.teamName ?? MemberMapper.noTeam,
      userId: u.id,
      email: u.email,
      teamId: t?.teamId ?? null,
      roleKey: t?.role ?? null,
      bindingVersion: t?.rowVersion ?? null,
      status: u.status,
      workspaceAdmin: u.workspaceAdmin,
      userVersion: u.rowVersion,
    };
  }

  static withBinding(member: TeamMember, b: RoleBindingDto): TeamMember {
    return { ...member, role: RoleKeys.toRole(b.role), roleKey: b.role, team: b.teamName, bindingVersion: b.rowVersion };
  }
}

/** Maps workspace settings; the row version becomes the If-Match token. */
export class WorkspaceSettingsMapper {
  static settings(dto: WorkspaceSettingsDto): WorkspaceSettings {
    return { aiAssistEnabled: dto.aiAssistEnabled, defaultRequireProdApproval: dto.defaultRequireProdApproval, version: dto.rowVersion };
  }
}
