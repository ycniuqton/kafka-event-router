import { PasswordRules, RoleKeys, UserStatus, type Access, type CreatedUser, type NewUserDraft, type PasswordResetDraft, type RoleKey, type TeamMember, type WorkspaceSettings } from '../../../domain';

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
  passwordLogin?: boolean;
  mustChangePassword?: boolean;
};

/** `POST /users` body (password mode). */
export type UserCreateDto = { email: string; displayName: string; password?: string; workspaceAdmin?: boolean; teamId?: string; role?: RoleKey };

/** `POST /users` response: the user and the one-time generated password. */
export type UserCreatedDto = { user: UserDto; generatedPassword?: string };

/** `POST /users/{id}/password-reset` response. */
export type PasswordResetDto = { generatedPassword?: string };

/** `RoleBinding` of `PUT /role-bindings/{userId}/{teamId}`. */
export type RoleBindingDto = { userId: string; teamId: string; teamName: string; role: RoleKey; rowVersion: number };

/** `WorkspaceSettings` of `GET/PATCH /workspace-settings`. */
export type WorkspaceSettingsDto = { aiAssistEnabled: boolean; defaultRequireProdApproval: boolean; routerCausedAlertDefault?: boolean; rowVersion: number };

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

  static passwordReset(userId: string): string {
    return `${AccessEndpoint.user(userId)}/password-reset`;
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
      passwordLogin: u.passwordLogin ?? false,
      mustChangePassword: u.mustChangePassword ?? false,
    };
  }

  static withBinding(member: TeamMember, b: RoleBindingDto): TeamMember {
    return { ...member, role: RoleKeys.toRole(b.role), roleKey: b.role, team: b.teamName, bindingVersion: b.rowVersion };
  }
}

/** Maps the create-user and password-reset drafts onto request bodies and their one-time passwords back. */
export class PasswordAdminMapper {
  static createBody(draft: NewUserDraft): UserCreateDto {
    const password = PasswordRules.chosen(draft.passwordSource, draft.password) ?? undefined;
    const team = draft.teamId ? { teamId: draft.teamId, role: draft.role } : {};
    return { email: draft.email.trim(), displayName: draft.displayName.trim(), password, workspaceAdmin: draft.workspaceAdmin, ...team };
  }

  static created(dto: UserCreatedDto): CreatedUser {
    return { members: MemberMapper.rows([dto.user]), generatedPassword: dto.generatedPassword ?? null };
  }

  static resetBody(draft: PasswordResetDraft): { password?: string } {
    const password = PasswordRules.chosen(draft.passwordSource, draft.password);
    return password === null ? {} : { password };
  }
}

/** Maps workspace settings; the row version becomes the If-Match token. */
export class WorkspaceSettingsMapper {
  static settings(dto: WorkspaceSettingsDto): WorkspaceSettings {
    return { aiAssistEnabled: dto.aiAssistEnabled, defaultRequireProdApproval: dto.defaultRequireProdApproval, routerCausedAlertDefault: dto.routerCausedAlertDefault, version: dto.rowVersion };
  }
}
