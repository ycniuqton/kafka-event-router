import { Role, type CurrentUser } from '../../../../domain';
import type { SessionUser, TeamRole } from './SessionService';

/** Maps `/auth/me` onto the shell's `CurrentUser` (highest role across teams; first team shown). */
export class CurrentUserMapper {
  static readonly noTeam = '—';
  private static readonly roles: Readonly<Record<TeamRole['role'], Role>> = {
    viewer: Role.Viewer,
    editor: Role.Editor,
    approver: Role.Approver,
    admin: Role.Admin,
  };
  private static readonly rank: readonly Role[] = [Role.Viewer, Role.Editor, Role.Approver, Role.Admin];

  static toCurrentUser(user: SessionUser): CurrentUser {
    return {
      name: user.display_name || user.email,
      initials: user.initials,
      workspace: user.workspace,
      team: user.teams[0]?.name ?? CurrentUserMapper.noTeam,
      role: CurrentUserMapper.topRole(user.teams),
    };
  }

  private static topRole(teams: TeamRole[]): Role {
    const ranks = teams.map((t) => CurrentUserMapper.rank.indexOf(CurrentUserMapper.roles[t.role]));
    return CurrentUserMapper.rank[Math.max(0, ...ranks)];
  }
}
